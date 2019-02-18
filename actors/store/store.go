// Package store simulates a generic business logic microservice
// Takes incoming traffic and calls into dependent microservices in a single zone
package store

import (
	"fmt"
	"github.com/adrianco/spigo/tooling/archaius"
	"github.com/adrianco/spigo/tooling/collect"
	"github.com/adrianco/spigo/tooling/flow"
	"github.com/adrianco/spigo/tooling/gotocol"
	"github.com/adrianco/spigo/tooling/handlers"
	"github.com/adrianco/spigo/tooling/names"
	"github.com/adrianco/spigo/tooling/ribbon"
	"time"
	"log"
	
)

// Start store, all configuration and state is sent via messages
func Start(listener chan gotocol.Message) {
	// remember the channel to talk to microservices
	microservices := ribbon.MakeRouter()
	dependencies := make(map[string]time.Time) // dependent services and time last updated
	store := make(map[string]string, 4)        // key value store
	store["why?"] = "because..."
	var netflixoss chan gotocol.Message                                           // remember creator and how to talk back to incoming requests
	var name string                                                               // remember my name
	eureka := make(map[string]chan gotocol.Message, len(archaius.Conf.ZoneNames)) // service registry per zone
	hist := collect.NewHist("")
	ep, _ := time.ParseDuration(archaius.Conf.EurekaPoll)
	eurekaTicker := time.NewTicker(ep)
	var delaytime time.Duration
	var delaysymbol int = 0
	for {
		select {
		case msg := <-listener:
			if msg.Imposition == gotocol.Put{
				flow.Instrument(msg, name, hist, "NO")
			}else if delaysymbol == 1 {
				log.Println("begin")
				time.Sleep(delaytime)
				log.Println("end")
				flow.Instrument(msg, name, hist, "YES")
				delaysymbol = 0
			}else{
				flow.Instrument(msg, name, hist, "NO")
			}
			switch msg.Imposition {
			case gotocol.Hello:
				if name == "" {
					// if I don't have a name yet remember what I've been named
					netflixoss = msg.ResponseChan // remember how to talk to my namer
					name = msg.Intention          // message body is my name
					hist = collect.NewHist(name)
				}
			case gotocol.Inform:
				eureka[msg.Intention] = handlers.Inform(msg, name, listener)
			case gotocol.NameDrop: // cross zone = true
				handlers.NameDrop(&dependencies, microservices, msg, name, listener, eureka, true)
			case gotocol.Forget:
				// forget a buddy
				handlers.Forget(&dependencies, microservices, msg)
			case gotocol.GetRequest:
				// return any stored value for this key
				outmsg := gotocol.Message{gotocol.GetResponse, listener, time.Now(), msg.Ctx, store[msg.Intention]}
				fmt.Println(msg.Intention,"TTTTTT",store[msg.Intention],"XXX~~~~~***////")
				flow.AnnotateSend(outmsg, name,"NO")
				outmsg.GoSend(msg.ResponseChan)
			case gotocol.GetResponse:
				// return path from a request, send payload back up (not currently used)
			case gotocol.Put:
				// set a key value pair and replicate to other stores
				var key , value string
				fmt.Sscanf(msg.Intention, "%s%s", &key, &value)
				log.Println(msg.Intention+"~~~~~~~~~~~~~~~~!!!@@@")
				log.Println(key,value)
				if key != "" && value != "" {
					store[key] = value
					// duplicate the request on to all connected store nodes with the same package name as this one
					for _, n := range microservices.All(names.Package(name)).Names() {
						outmsg := gotocol.Message{gotocol.Replicate, listener, time.Now(), msg.Ctx.NewParent(), msg.Intention}
						flow.AnnotateSend(outmsg, name,"NO")
						outmsg.GoSend(microservices.Named(n))
					}
				}
			case gotocol.Delay:
				delaysymbol = 1
				d, e := time.ParseDuration(msg.Intention)
				if e == nil && d >= time.Millisecond && d <= time.Hour {
					delaytime = d
				}
				// log.Println("begin")
				// time.Sleep(delaytime)
				// delaysymbol = 0
				// log.Println("end")
			case gotocol.Replicate:
				// Replicate is used between store nodes
				// end point for a request
				var key, value string
				fmt.Sscanf(msg.Intention, "%s%s", &key, &value)
				// log.Printf("store: %v:%v", key, value)
				if key != "" && value != "" {
					store[key] = value
				}
			case gotocol.Goodbye:
				gotocol.Message{gotocol.Goodbye, nil, time.Now(), gotocol.NilContext, name}.GoSend(netflixoss)
				return
			}
		case <-eurekaTicker.C: // check to see if any new dependencies have appeared
			for {//这一部分是否多余(select 好像可以保证一次只有一个case在执行)或者不够合理(也许会产生竞争)，
				if delaysymbol == 0 {
					break
				}
			}
			for dep := range dependencies {
				for _, ch := range eureka {
					ch <- gotocol.Message{gotocol.GetRequest, listener, time.Now(), gotocol.NilContext, dep}
				}
			}
		}
	}
}
