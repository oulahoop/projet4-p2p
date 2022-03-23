package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Tout ce code est ajouté pour le mini-projet 3

type chatMessage struct {
	from    string
	content string
	private bool
}

type peerCollection struct {
	peers  []peer
	access sync.RWMutex
}

// Lit les messages sur la connexion du pair p et recopie
// ces messages dans le canal receiveChan (ce qui permettra
// ensuite de les afficher)
func (p peer) receiveMessages(receiveChan chan chatMessage) {

	reader := bufio.NewReader(p.conn)

	for {
		msg, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				log.Print(p.addr, " est parti.")
				return
			}
			log.Print("Erreur de réception d'un message provenant de ", p.addr)
		}

		// Les messages reçus sont de la forme type:::message où type
		// vaut "private" pour les messages privés
		splitMsg := strings.Split(msg, ":::")

		receiveChan <- chatMessage{
			from:    p.addr,
			content: splitMsg[1],
			private: splitMsg[0] == "private",
		}
	}

}

// Récupère les messages dans le canal toSend de p et les envoie
// au pair correspondant via le réseau no rappelle que les lectures
// et écritures dans des canaux (chan) se font sans soucis liés aux
// accès concurrents aux ressources, il n'est donc pas la peine
// d'ajouter un verou
func (p peer) sendMessages() {

	writer := bufio.NewWriter(p.conn)

	for {
		msg := <-p.toSend

		protocolMsg := msg.content
		if msg.private {
			protocolMsg = "private:::" + protocolMsg
		} else {
			protocolMsg = "public:::" + protocolMsg
		}

		log.Print("J'envoie ceci : '", protocolMsg[:len(protocolMsg)-1], "' à ", p.addr)

		_, err1 := writer.WriteString(protocolMsg)
		err2 := writer.Flush()
		if err1 != nil || err2 != nil {
			log.Print("Echec de l'envoie d'un message à ", p.addr)
		}
	}
}

// Récupère les messages entrés au clavier par l'utilisateur et les
// place dans les canneaux des bons pairs pour envoie futur
func (peers *peerCollection) prepareChatMessages() {

	reader := bufio.NewReader(os.Stdin)

	for {
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Print("Erreur, le message entré n'est pas correct")
			continue
		}

		log.Print("Sur l'entrée standard j'ai lu ", input[:len(input)-1])

		splitInput := strings.Split(input, ":")

		// on reserve l'accès à la structure peers en lecture
		// c'est ok car on ne va pas modifier cette structure
		// mais seulement écrire dans un canal
		peers.access.RLock()

		if peerID, err := strconv.Atoi(splitInput[0]); len(splitInput) >= 2 && err == nil && peerID < len(peers.peers) {
			peers.peers[peerID].toSend <- chatMessage{
				content: strings.Join(splitInput[1:], ""),
				private: true,
			}
		} else {
			msg := chatMessage{content: strings.Join(splitInput, "")}
			for peerID := range peers.peers {
				peers.peers[peerID].toSend <- msg
			}
		}

		// on a terminé d'accéder à peers, on libère l'accès
		peers.access.RUnlock()
	}

}

// Traite les messages du canal receiveChan pour les afficher sur
// la sortie standard
func displayChatMessages(receiveChan chan chatMessage) {
	for {
		msg := <-receiveChan
		if msg.private {
			fmt.Print(
				"Message privé de ", msg.from, ": ",
				msg.content,
			)
		} else {
			fmt.Print(
				"Message de ", msg.from, ": ",
				msg.content,
			)
		}
	}
}
