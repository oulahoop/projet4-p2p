package main

import (
	"bufio" // pour simplifier les lectures/écritures sur le réseau
	"flag"  // pour simplifier la lecture de la ligne de commande
	"fmt"   // pour construire des chaînes de caratères
	"log"   // pour afficher des informations sur le déroulement du programme
	"net"   // pour réaliser la connexion réseau
	"strconv"
	"strings" // pour manipuler des chaînes de caractères
	"sync"
)

// Le type peer représente la connaissance qu'on a d'un pair du réseau, pour
// le moment c'est uniquement la connexion (conn) ouverte avec lui et son
// adresse ip avec le port pris en compte (addr)
type peer struct {
	conn   net.Conn
	addr   string
	toSend chan chatMessage // AJOUT MINI-PROJET 3
}

type clock struct {
	clockMap map[string]int
	myPort   string
	access   sync.RWMutex
}

func main() {
	// Lecture des arguments de la ligne de commande : un port obligatoire, une
	// adresse et un port facultatifs, un booléen first qui permet de préciser
	// si on souhaite créer un nouveau réseau ou pas (et donc rejoindre un réseau)
	// existant
	port := flag.Int(
		"port", 8080,
		"Port sur lequel écouter")
	isFirst := flag.Bool(
		"first", false,
		"Pour déclencher la création d'un nouveau réseau")
	contactaddr := flag.String(
		"contactaddr", "127.0.0.1",
		"Adresse de contact quand on ne crée pas un nouveau réseau")
	contactport := flag.Int(
		"contactport", 8080,
		"Port de contact quand on ne crée pas un nouveau réseau")

	flag.Parse()

	// La structure de donnée dans laquelle on mémorise les informations sur
	// les autres pairs du réseau
	// MODIFICATION MINI-PROJET 3
	otherPeers := peerCollection{
		peers: make([]peer, 0),
	}

	// AJOUT MINI-PROJET 3
	// Un canal pour mettre les messages reçus de tous les autres pairs
	// afin de les afficher
	receiveChan := make(chan chatMessage, 10)

	//Horloge
	myClock := clock{
		clockMap: map[string]int{"127.0.0.1:" + strconv.Itoa(*port): 0},
		myPort:   strconv.Itoa(*port),
	}

	//moi-même
	myClock.access.RLock()
	log.Print("CLOCK --- Initialisation : ", myClock.clockMap)
	myClock.access.RUnlock()

	// Si on n'est pas le premier, on rentre en contact avec notre point d'entrée
	// sur le réseau pour lui demander les informations sur les autres pairs
	if !(*isFirst) {
		dialAddr := fmt.Sprint(*contactaddr, ":", *contactport)
		log.Print("Je ne suis pas le premier arrivé, j'essaye de contacter ", dialAddr)

		// Connexion au point d'entrée
		conn, err := net.Dial("tcp", dialAddr)
		if err != nil {
			log.Fatal("Impossible de se connecter à l'adresse ", dialAddr)
		}
		myClock.access.Lock()
		myClock.clockMap[dialAddr] = 0
		myClock.access.Unlock()
		// Demande des autres pairs
		// MODIFICATION MINI-PROJET 3
		otherPeers.peers = handleArrival(conn, *port, &myClock)

		// Constitution du réseau en y ajoutant le point d'entrée
		// MODIFICATION MINI-PROJET 3
		otherPeers.peers = append(otherPeers.peers, peer{
			conn:   conn,
			addr:   dialAddr,
			toSend: make(chan chatMessage, 10),
		})

		log.Print("Je suis bien entré dans le réseau")
		// MODIFICATION MINI-PROJET 3
		displayNetwork(otherPeers.peers)
	}
	myClock.access.RLock()
	log.Print("CLOCK --- Après avoir rejoint le réseau : ", myClock)
	myClock.access.RUnlock()

	// AJOUT MINI-PROJET 3
	// Pour chaque pair du réseau on lance la routine qui permettra de
	// lui envoyer des messages et la goroutine qui permettra de recevoir ses messages
	for _, peer := range otherPeers.peers {
		go peer.receiveMessages(receiveChan, &myClock)
		go peer.sendMessages(&myClock)
	}

	// AJOUT MINI-PROJET 3
	// On démarre les routines qui vont permettre d'écrire des messages pour les
	// autres pairs et d'afficher les messages reçus
	go otherPeers.prepareChatMessages(&myClock)
	go displayChatMessages(receiveChan, &myClock)

	// Après avoir pris connaissance des autres, on se met en écoute
	listenPort := fmt.Sprint(":", *port)
	listener, err := net.Listen("tcp", listenPort)
	if err != nil {
		log.Fatal("Impossible d'écouter à l'adresse ", listenPort)
	}

	log.Print("Je suis prêt à accepter les demandes de connexion des nouveaux")
	for {
		// On traite entièrement l'arrivée d'un pair avant d'être prêt à en accepter
		// un autre (ceci était l'une des hypothèses de l'énoncé, en pratique il
		// faudrait être un peu plus souple)
		log.Print("Je me mets en écoute sur ", listenPort)
		conn, err := listener.Accept()
		if err != nil {
			log.Print("Problème à l'arrivée d'un pair")
			continue
		}
		log.Print("Un nouveau pair vient de me contacter")
		ip, ok := handleConnection(conn, &otherPeers, &myClock)
		if ok {
			// MODIFICATION MINI-PROJET 3
			// réservation en écriture de la ressource avant de la modifier
			newPeer := peer{
				conn:   conn,
				addr:   ip,
				toSend: make(chan chatMessage, 10),
			}

			otherPeers.access.Lock()
			otherPeers.peers = append(otherPeers.peers, newPeer)
			otherPeers.access.Unlock()

			log.Print("Le nouveau pair a été intégré, son adresse est ", ip)

			// on peut réserver la ressource en lecture ici, mais c'est inutile
			// car la seule écriture possible est dans le même fil d'exécution
			displayNetwork(otherPeers.peers)

			// AJOUT MINI-PROJET 3
			// Une fois le pair intégré dans le réseau, on lance la goroutine
			// qui permettra de lui envoyer des messages et la goroutine qui
			// permettra de recevoir ses messages
			go newPeer.receiveMessages(receiveChan, &myClock)
			go newPeer.sendMessages(&myClock)
			myClock.access.RLock()
			log.Print("CLOCK --- Nouvelle utilisateur ayant rejoint, j'update mon horloge : ", myClock)
			myClock.access.RUnlock()
		}

	}

}

// handleConnection gère une demande de connexion d'un nouveau pair
// MODIFICATION MINI-PROJET 3 : otherPeers n'est plus seulement un tableau de peer
func handleConnection(conn net.Conn, otherPeers *peerCollection, myClock *clock) (string, bool) {

	reader := bufio.NewReader(conn)

	// En arrivant, le pair doit nous envoyer un message. Ici on le lit.
	msg, err := reader.ReadString('\n')
	if err != nil {
		log.Print("Problème à l'arrivée d'un pair (réception de message)")
		return "", false
	}

	// Le protocole choisi est tel qu'un message doit toujours commencer
	// par une chaîne de caractère indiquant la nature du nouveau pair (Pour
	// qui nous sommes le point d'entrée ou bien qui a déjà été intégré au
	// réseau par quelqu'un d'autre), suive de :::, suivie d'une information
	// sur le pair
	splitMsg := strings.Split(msg, ":::")

	switch splitMsg[0] {
	case "arrival":
		// le début du message est "arrival", indiquant que nous somme le point
		// d'entrée du nouveau père
		log.Print("C'est moi qui gère l'arrivée de ce nouveau pair")
		writer := bufio.NewWriter(conn)
		// on doit envoyer l'adresse de tous les autres pairs au nouveau

		// avant de parcourir otherPeers on réserve la structure en lecture
		otherPeers.access.RLock()

		for _, peer := range otherPeers.peers {
			toSend := fmt.Sprint(
				"peer:::",
				peer.addr,
				"\n",
			)
			_, err := writer.WriteString(toSend)
			if err != nil {
				log.Print("L'information sur le pair ", peer.addr, "n'a pas pu être envoyée")
			}
		}

		// quand le parcours de otherPeers est terminé on libère
		otherPeers.access.RUnlock()

		_, err1 := writer.WriteString("done\n")
		err2 := writer.Flush()
		if err1 != nil || err2 != nil {
			log.Print("Problème lors de l'envoie des informations sur les autres pairs")
		}
	case "connection":
		// le début du message est "connection", indiquant que nous ne sommes pas
		// le point d'entrée du nouveau père
		log.Print("Un autre a déjà géré l'arrivée de ce nouveau pair")
	}

	// On récupère l'adresse du nouveau pair pour pouvoir la retourner puis
	// l'ajouter à notre tableau de pairs (otherPeers)
	ip := conn.RemoteAddr().String()
	splitPoint := strings.LastIndex(ip, ":")
	ip = ip[:splitPoint]
	ip += ":" + splitMsg[1]
	ip = strings.TrimSuffix(ip, "\n")
	myClock.access.Lock()
	myClock.clockMap[ip] = 0
	myClock.access.Unlock()
	return ip, true
}

// handleArrival gère notre arrivée dans le réseau : prise de contact avec
// notre point d'entrée, récupération des adresses de tous les autres pairs
// MODIFICATION MINI-PROJET 3 : un argument ajouté, receiveChan
func handleArrival(conn net.Conn, myPort int, myClock *clock) []peer {

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// pour signaler qu'on est nouveau on envoie un message "arrival:::port"
	toSend := fmt.Sprint("arrival:::", myPort, "\n")
	_, err1 := writer.WriteString(toSend)
	err2 := writer.Flush()
	if err1 != nil || err2 != nil {
		log.Fatal("Impossible d'écrire à mon point d'entrée, tant pis, j'arrête ici")
	}

	// création d'un tableau de pair pour stocker les infos sur les autres,
	// sera retourné à la fin pour être intégré dans notre tableau de pairs
	// (otherPeers)
	peers := make([]peer, 0)

loop:
	for {
		msg, err := reader.ReadString('\n')
		if err != nil {
			log.Print("J'ai mal lu un message, je rate peut-être un autre pair")
			continue
		}

		// on boucle tant qu'on ne reçoit pas "done"
		// on traite des messages de la forme "peer:::addr" (dans ce cas on ajoute
		// un pair a peers) et on ignore tout le reste
		splitMsg := strings.Split(msg, ":::")

		switch splitMsg[0] {
		case "peer":
			fullAddr := strings.TrimSuffix(splitMsg[1], "\n")
			// prise de contact avec le nouveau pair
			newConn := getInTouch(fullAddr, myPort)
			if newConn != nil {
				newPeer := peer{
					conn:   newConn,
					addr:   fullAddr,
					toSend: make(chan chatMessage, 10),
				} // AJOUT MINI-PROJET 3
				peers = append(peers, newPeer)
				myClock.access.Lock()
				myClock.clockMap[fullAddr] = 0
				myClock.access.Unlock()
			}
		case "done\n":
			break loop
		}
	}

	return peers
}

// getInTouch permet d'ouvrir une connexion avec un pair dont on vient
// d'obtenir l'adresse
func getInTouch(fullAddr string, myPort int) net.Conn {

	log.Print("On vient de me donner l'adresse ", fullAddr, ", j'ouvre tout de suite une connexion !")
	conn, err := net.Dial("tcp", fullAddr)
	if err != nil {
		log.Print("Attention : impossible de joindre ", fullAddr)
		return nil
	}

	// quand on ouvre une connexion avec quelqu'un, on lui transmet aussi notre
	// port d'écoute car il ne peut pas le deviner (il est différent du port
	// utiliser pour la connexion)
	writer := bufio.NewWriter(conn)
	msg := fmt.Sprint("connection:::", myPort, "\n")
	_, err1 := writer.WriteString(msg)
	err2 := writer.Flush()
	if err1 != nil || err2 != nil {
		log.Print("Attention : impossible de se signaler à ", fullAddr)
	}

	return conn
}

// displayNetwork affiche les pairs actuellement connus
// MODIFICATION MINI-PROJET 3 : affichage de l'ID du pair
func displayNetwork(peers []peer) {
	addr := ""
	for peerID, peer := range peers {
		addr += fmt.Sprint(
			"\t", peer.addr,
			" (", peerID, ")\n",
		)
	}
	log.Print("Les adresses des autres pairs du réseau sont\n", addr)
}
