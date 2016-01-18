package coapmq

import (
	"log"
	"net"

	"github.com/dustin/go-coap"
)

type chanMapStringList map[*net.UDPAddr][]string
type stringMapChanList map[string][]*net.UDPAddr

type Broker struct {
	capacity int

	msgIndex uint16 //for increase and sync message ID

	//map to store "chan -> Topic List" for find subscription
	clientMapTopics chanMapStringList
	//map to store "topic -> chan List" for publish
	topicMapClients stringMapChanList
}

//Create a new pubsub server using CoAP protocol
//maxChannel: It is the subpub topic limitation size, suggest not lower than 1024 for basic usage
func NewBroker(maxChannel int) *Broker {
	cSev := new(Broker)
	cSev.capacity = maxChannel
	cSev.clientMapTopics = make(map[*net.UDPAddr][]string, maxChannel)
	cSev.topicMapClients = make(map[string][]*net.UDPAddr, maxChannel)
	cSev.msgIndex = GetIPv4Int16() + GetLocalRandomInt()
	log.Println("Init msgID=", cSev.msgIndex)
	return cSev
}

func (c *Broker) genMsgID() uint16 {
	c.msgIndex = c.msgIndex + 1
	return c.msgIndex
}

func (c *Broker) removeSubscription(topic string, client *net.UDPAddr) {
	removeIndexT2C := -1
	if val, exist := c.topicMapClients[topic]; exist {
		for k, v := range val {
			if v == client {
				removeIndexT2C = k
			}
		}
		if removeIndexT2C != -1 {
			sliceClients := c.topicMapClients[topic]
			if len(sliceClients) > 1 {
				c.topicMapClients[topic] = append(sliceClients[:removeIndexT2C], sliceClients[removeIndexT2C+1:]...)
			} else {
				delete(c.topicMapClients, topic)
			}
		}
	}

	removeIndexC2T := -1
	if val, exist := c.clientMapTopics[client]; exist {
		for k, v := range val {
			if v == topic {
				removeIndexC2T = k
			}
		}
		if removeIndexC2T != -1 {
			sliceTopics := c.clientMapTopics[client]
			if len(sliceTopics) > 1 {
				c.clientMapTopics[client] = append(sliceTopics[:removeIndexC2T], sliceTopics[removeIndexC2T+1:]...)
			} else {
				delete(c.clientMapTopics, client)
			}
		}
	}

}

func (c *Broker) addSubscription(topic string, client *net.UDPAddr) {
	topicFound := false
	if val, exist := c.topicMapClients[topic]; exist {
		for _, v := range val {
			if v == client {
				topicFound = true
			}
		}
	}
	if topicFound == false {
		c.topicMapClients[topic] = append(c.topicMapClients[topic], client)
	}

	clientFound := false
	if val, exist := c.clientMapTopics[client]; exist {
		for _, v := range val {
			if v == topic {
				clientFound = true
			}
		}
	}

	if clientFound == false {
		c.clientMapTopics[client] = append(c.clientMapTopics[client], topic)
	}
}

func (c *Broker) publish(l *net.UDPConn, topic string, msg string) {
	if clients, exist := c.topicMapClients[topic]; !exist {
		return
	} else { //topic exist, publish it
		for _, client := range clients {
			c.publishMsg(l, client, topic, msg)
			log.Println("topic->", topic, " PUB to ", client, " msg=", msg)
		}
	}
	log.Println("pub finished")
}

func (c *Broker) handleCoAPMessage(l *net.UDPConn, a *net.UDPAddr, m *coap.Message) *coap.Message {
	var topic string
	if m.Path() == nil {
		return nil
	}

	cmd := m.Path()[0]
	//"ps" is mandantory command for coapmq
	if cmd != "ps" {
		return nil
	}

	if len(m.Path()) > 1 {
		topic = m.Path()[1]
	}

	log.Println("cmd=", cmd, " topic=", topic, " msg=", string(m.Payload), "code=", m.Code)

	if cmd == "ADDSUB" {
		log.Println("add sub topic=", topic, " in client=", a)
		c.addSubscription(topic, a)
		c.responseOK(l, a, m)
	} else if cmd == "REMSUB" {
		log.Println("remove sub topic=", topic, " in client=", a)
		c.removeSubscription(topic, a)
		c.responseOK(l, a, m)
	} else if cmd == "PUB" {
		c.publish(l, topic, string(m.Payload))
		c.responseOK(l, a, m)
	} else if cmd == "HB" {
		//For heart beat request just return OK
		log.Println("Got heart beat from ", a)
		c.responseOK(l, a, m)
	}

	for k, v := range c.topicMapClients {
		log.Println("Topic=", k, " sub by client=>", v)
	}
	return nil
}

//Start to listen udp port and serve request, until faltal eror occur
func (c *Broker) ListenAndServe(udpPort string) {
	log.Fatal(coap.ListenAndServe("udp", udpPort,
		coap.FuncHandler(func(l *net.UDPConn, a *net.UDPAddr, m *coap.Message) *coap.Message {
			return c.handleCoAPMessage(l, a, m)
		})))
}

func (c *Broker) responseOK(l *net.UDPConn, a *net.UDPAddr, m *coap.Message) {
	m2 := coap.Message{
		Type:      coap.Acknowledgement,
		Code:      coap.GET,
		MessageID: m.MessageID,
		Payload:   m.Payload,
	}

	//m2.SetOption(coap.ContentFormat, coap.TextPlain)
	m2.SetOption(coap.ContentFormat, coap.AppLinkFormat)
	m2.SetPath(m.Path())
	err := coap.Transmit(l, a, m2)
	if err != nil {
		log.Printf("Error on transmitter, stopping: %v", err)
		return
	}
}

func (c *Broker) publishMsg(l *net.UDPConn, a *net.UDPAddr, topic string, msg string) {
}