// Package monolith simulates a monolithic business logic microservice
// Takes incoming traffic and calls into dependent microservices across all zones
package monolith

import (
	"github.com/chenhy/spigo/tooling/archaius"
	"github.com/chenhy/spigo/tooling/collect"
	"github.com/chenhy/spigo/tooling/flow"
	"github.com/chenhy/spigo/tooling/gotocol"
	"github.com/chenhy/spigo/tooling/handlers"
	"github.com/chenhy/spigo/tooling/ribbon"
	// "github.com/chenhy/spigo/tooling/names"
	"time"
	"log"
	"strings"
	"fmt"
)

// Start - all configuration and state is sent via messages
func Start(listener chan gotocol.Message) {
	microservices := ribbon.MakeRouter()
	dependencies := make(map[string]time.Time)         // dependent services and time last updated
	//所有package的parent 是 package eureka
	var parent chan gotocol.Message                    // remember how to talk back to creator
	requestor := make(map[string]gotocol.Routetype)    // remember where requests came from when responding
	var name string                                    // remember my name
	eureka := make(map[string]chan gotocol.Message, 1) // service registry
	http_status := make(map[string]int)				   // record http_connection_status
	hist := collect.NewHist("")
	ep, _ := time.ParseDuration(archaius.Conf.EurekaPoll)
	eurekaTicker := time.NewTicker(ep)
	var delaytime time.Duration
	var delaysymbol int = 0
	var exit_symbol int = 0
	for {
		select {
		case msg := <-listener:
			if msg.Imposition == gotocol.Final{
				gotocol.Message{gotocol.Final, nil, time.Now(), gotocol.NilContext, name}.GoSend(parent)
				return
			}
			if exit_symbol == 1 {
				flow.Instrument(msg, name, hist, "DONE")
				flow.Add2Buffer(msg)
				continue
			}
			if msg.Imposition == gotocol.Put{
				flow.Instrument(msg, name, hist, "NO")
			}else if delaysymbol == 1 {
				log.Println("begin")
				fmt.Println("*******",delaysymbol)
				time.Sleep(delaytime)
				log.Println("end")
				flow.Instrument(msg, name, hist, "YES"+name)
				delaysymbol = 0
			}else{
				flow.Instrument(msg, name, hist, "NO")
			}
			switch msg.Imposition {
			case gotocol.Hello:
				if name == "" {
					// if I don't have a name yet remember what I've been named
					parent = msg.ResponseChan // remember how to talk to my namer
					name = msg.Intention      // message body is my name
					hist = collect.NewHist(name)
				}
			case gotocol.Inform:
				eureka[msg.Intention] = handlers.Inform(msg, name, listener)
			case gotocol.NameDrop:
				if strings.Contains(msg.Intention,"."){
					http_status[msg.Intention] = 200
				}
				handlers.NameDrop(&dependencies, microservices, msg, name, listener, eureka, true) // true to setup cross region routing
			case gotocol.Forget:
				// forget a buddy
				http_status[msg.Intention] = 503
				handlers.Forget(&dependencies, microservices, msg)
			case gotocol.GetRequest:
				// route the request on to microservices
				handlers.GetRequest(msg, name, listener, &requestor, microservices)
				// log.Println(names.Service(microservices.Names()[0]),"~~~~~~~~~~~~~|||~~~~~~~~~12")
			case gotocol.GetResponse:
				// return path from a request, send payload back up using saved span context - server send
				handlers.GetResponse(msg, name, listener, &requestor)
			case gotocol.Put:
				// route the request on to a random dependency
				
				handlers.Put(msg, name, listener, &requestor, microservices)
			case gotocol.Delay:
				delaysymbol = 1

				d, e := time.ParseDuration(msg.Intention)
				if e == nil && d >= time.Millisecond && d <= time.Hour {
					delaytime = d
				}
				// fmt.Println(delaytime"DELAY,**********____")
				// if archaius.Conf.RunToEnd{
					flow.Add2Buffer(msg)
				// }
				// log.Println("begin")
				// time.Sleep(delaytime)
				// delaysymbol = 0
				// log.Println("end")
			case gotocol.Goodbye:
				for _, ch := range eureka { // tell name service I'm not going to be here
					
					ch <- gotocol.Message{gotocol.Delete, nil, time.Now(), gotocol.NilContext, name}
				}
				gotocol.Message{gotocol.Final, nil, time.Now(), gotocol.NilContext, name}.GoSend(parent)
				flow.Add2Buffer(msg)
				exit_symbol = 1
				//return
			}
		case <-eurekaTicker.C: // check to see if any new dependencies have appeared
			//for {//这一部分是否多余(select 好像可以保证一次只有一个case在执行)或者不够合理(也许会产生竞争)，
			//	if delaysymbol == 0{
			//		continue
			//	}
			//}
			for dep := range dependencies {
				for _, ch := range eureka {
					ch <- gotocol.Message{gotocol.GetRequest, listener, time.Now(), gotocol.NilContext, dep}
				}
			}
		}
	}
}
