package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Tout ce code est ajouté pour le mini-projet 3

type chatMessage struct {
	from      string
	content   string
	fromClock map[string]int
	private   bool
}

type peerCollection struct {
	peers  []peer
	access sync.RWMutex
}

// Lit les messages sur la connexion du pair p et recopie
// ces messages dans le canal receiveChan (ce qui permettra
// ensuite de les afficher)
func (p peer) receiveMessages(receiveChan chan chatMessage, myClock *clock) {
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
		myClock.access.Lock()
		myClock.clockMap[fmt.Sprint("127.0.0.1:", myClock.myPort)] += 1
		myClock.access.Unlock()
		// Les messages reçus sont de la forme type:::message où type
		// vaut "private" pour les messages privés
		//type:::clock:::message
		//type clock message
		//clock -> clock
		//garde la plus ancienne
		splitMsg := strings.Split(msg, ":::")

		clockMap := map[string]int{}
		for _, row := range strings.Split(splitMsg[1], ";") {
			split := strings.Split(row, ",")
			clockMap[split[0]], _ = strconv.Atoi(split[1])
		}

		receiveChan <- chatMessage{
			from:      p.addr,
			content:   splitMsg[2],
			fromClock: clockMap,
			private:   splitMsg[0] == "private",
		}
	}

}

// Récupère les messages dans le canal toSend de p et les envoie
// au pair correspondant via le réseau no rappelle que les lectures
// et écritures dans des canaux (chan) se font sans soucis liés aux
// accès concurrents aux ressources, il n'est donc pas la peine
// d'ajouter un verou
func (p peer) sendMessages(myClock *clock) {

	writer := bufio.NewWriter(p.conn)

	for {
		msg := <-p.toSend

		//On met l'horloge du pair acutel en string pour le protocole
		protocoleClock := ""
		myClock.access.RLock()
		for index, value := range myClock.clockMap {
			protocoleClock += index + "," + strconv.Itoa(value) + ";"
		}
		myClock.access.RUnlock()
		//on retire le dernier ";"
		protocoleClock = protocoleClock[:len(protocoleClock)-1]
		protocolMsg := msg.content

		//protocole : type:::clock:::message
		if msg.private {
			protocolMsg = "private:::" + protocoleClock + ":::" + protocolMsg
		} else {
			protocolMsg = "public:::" + protocoleClock + ":::" + protocolMsg
		}

		timeSleeping := rand.Intn(10)
		log.Print("J'envoie ceci : '", protocolMsg[:len(protocolMsg)-1], "' à ", p.addr, " avec un délai de ", timeSleeping, " secondes.")
		time.Sleep(time.Second * time.Duration(timeSleeping))
		_, err1 := writer.WriteString(protocolMsg)
		err2 := writer.Flush()
		if err1 != nil || err2 != nil {
			log.Print("Echec de l'envoie d'un message à ", p.addr)
		}
	}
}

// Récupère les messages entrés au clavier par l'utilisateur et les
// place dans les canneaux des bons pairs pour envoie futur
func (peers *peerCollection) prepareChatMessages(myClock *clock) {

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
		myClock.access.Lock()
		myClock.clockMap["127.0.0.1:"+myClock.myPort] += 1
		myClock.access.Unlock()
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
func displayChatMessages(receiveChan chan chatMessage, myClock *clock) {
	msg := chatMessage{}
	for {
		msgToCompare := chatMessage{}
		if reflect.DeepEqual(msg, chatMessage{}) {
			msg = <-receiveChan
		}

		msgToCompare = <-receiveChan

		//compare
		msg, msgToCompare = compareClockMap(msg, msgToCompare)

		//update de myclock
		for index, value := range msgToCompare.fromClock {
			if myClock.clockMap[index] <= value {
				myClock.clockMap[index] = value
			}
		}

		//prendre en compte le message
		if msgToCompare.private {
			fmt.Print(
				"Message privé de ", msgToCompare.from, ": ",
				msgToCompare.content,
			)
		} else {
			fmt.Print(
				"Message de ", msgToCompare.from, ": ",
				msgToCompare.content,
			)
		}
	}
}

func compareClockMap(message chatMessage, messageToCompare chatMessage) (chatMessage, chatMessage) {
	isBefore := false
	for index, value := range message.fromClock {
		valueToCompare := messageToCompare.fromClock[index]
		if value <= valueToCompare {
			isBefore = true
		}
	}
	if isBefore {
		return messageToCompare, message
	}
	return message, messageToCompare
}
