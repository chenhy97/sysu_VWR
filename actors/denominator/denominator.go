// Package denominator simulates a global DNS service
// Takes incoming traffic and spreads it over elb's in multiple regions
package denominator

import (
	"fmt"
	"github.com/adrianco/spigo/tooling/archaius"
	"github.com/adrianco/spigo/tooling/collect"
	"github.com/adrianco/spigo/tooling/flow"
	"github.com/adrianco/spigo/tooling/gotocol"
	"github.com/adrianco/spigo/tooling/handlers"
	"github.com/adrianco/spigo/tooling/ribbon"
	"log"
	"math/rand"
	"time"
)

// Start the denominator, all configuration and state is sent via messages
func Start(listener chan gotocol.Message) {
	microservices := ribbon.MakeRouter()
	dependencies := make(map[string]time.Time)                                                          // dependent services and time last updated
	var parent chan gotocol.Message                                                                     // remember how to talk back to creator
	var name string                                                                                     // remember my name
	nethist := collect.NewHist("")                                                                      // don't know name yet - message network latency
	resphist := collect.NewHist("")                                                                     // response time history
	servhist := collect.NewHist("")                                                                     // service time history
	rthist := collect.NewHist("")                                                                       // round trip history
	eureka := make(map[string]chan gotocol.Message, len(archaius.Conf.ZoneNames)*archaius.Conf.Regions) // service registry per zone and region
	var chatrate time.Duration
	ep, _ := time.ParseDuration(archaius.Conf.EurekaPoll)
	eurekaTicker := time.NewTicker(ep)
	chatTicker := time.NewTicker(time.Hour)
	chatTicker.Stop()
	var delaysymbol int = 0
	var exit_symbol int = 0
	var delaytime time.Duration
	w := 1 // counter for random messages
	for {
		select {
		case msg := <-listener:
			if msg.Imposition == gotocol.Final {
				if archaius.Conf.Msglog {
					log.Printf("%v: Going away, was chatting every %v\n", name, chatrate)
				}
				collect.SaveHist(nethist, name, "_net")
				collect.SaveHist(resphist, name, "_resp")
				collect.SaveHist(servhist, name, "_serv")
				collect.SaveHist(rthist, name, "_rt")
				collect.SaveAllGuesses(name)
				gotocol.Message{gotocol.Final, nil, time.Now(), gotocol.NilContext, name}.GoSend(parent)
				return
			}
			if exit_symbol == 1{
				flow.Add2Buffer(msg)
				continue
			}
			if msg.Imposition == gotocol.Put{
				flow.Instrument(msg, name, nethist, "NO")
			}else if delaysymbol == 1 {
				log.Println("begin")
				time.Sleep(delaytime)
				log.Println("end")
				flow.Instrument(msg, name, nethist, "YES")
				delaysymbol = 0
			}else{
				flow.Instrument(msg, name, nethist, "NO")
			}
			switch msg.Imposition {
			case gotocol.Hello:
				if name == "" {
					// if I don't have a name yet remember what I've been named
					parent = msg.ResponseChan // remember how to talk to my namer
					name = msg.Intention      // message body is my name
					nethist = collect.NewHist(name + "_net")
					resphist = collect.NewHist(name + "_resp")
					servhist = collect.NewHist(name + "_serv")
					rthist = collect.NewHist(name + "_rt")
				}
			case gotocol.Inform:
				eureka[msg.Intention] = handlers.Inform(msg, name, listener)
			case gotocol.NameDrop:
				handlers.NameDrop(&dependencies, microservices, msg, name, listener, eureka, true)
			case gotocol.Forget:
				// forget a buddy
				handlers.Forget(&dependencies, microservices, msg)
			case gotocol.Delay:
				delaysymbol = 1
				d, e := time.ParseDuration(msg.Intention)
				if e == nil && d >= time.Millisecond && d <= time.Hour {
					delaytime = d
				}
				flow.Add2Buffer(msg)
				// log.Println("begin")
				// time.Sleep(delaytime)
				// delaysymbol = 0
				// log.Println("end")
			case gotocol.Chat:
				// setup the ticker to run at the specified rate
				d, e := time.ParseDuration(msg.Intention)
				if e == nil && d >= time.Millisecond && d <= time.Hour {
					chatrate = d
					chatTicker = time.NewTicker(chatrate)
				}
			case gotocol.GetResponse:
				// return path from a request, terminate and log response time in histograms
				flow.End(msg, resphist, servhist, rthist)
				flow.Add2Buffer(msg)
			case gotocol.Goodbye:
				
				//log.Println(parent)
				gotocol.Message{gotocol.Final, nil, time.Now(), gotocol.NilContext, name}.GoSend(parent)
				flow.Add2Buffer(msg)
				exit_symbol = 1
				// return
			}
		case <-eurekaTicker.C: // check to see if any new dependencies have appeared
			//for {//这一部分是否多余(select 好像可以保证一次只有一个case在执行)或者不够合理(也许会产生竞争)，
			//	if delaysymbol == 0 {
			//		break
			//	}
			//}
			for dep := range dependencies {
				for _, ch := range eureka {
					ch <- gotocol.Message{gotocol.GetRequest, listener, time.Now(), gotocol.NilContext, dep}
				}
			}
		case <-chatTicker.C:
			//for {//这一部分是否多余(select 好像可以保证一次只有一个case在执行)或者不够合理(也许会产生竞争)，
			//	if delaysymbol == 0 {
			//		break
			//	}
			//}
			c := microservices.Random()
			if c != nil {
				ctx := gotocol.NewTrace()
				now := time.Now()
				var sm gotocol.Message
				switch rand.Intn(3) {
				case 0:
					sm = gotocol.Message{gotocol.GetRequest, listener, now, ctx, "why?"}
				case 1:
					q := rand.Intn(w) // pick a random key that has already been put
					sm = gotocol.Message{gotocol.GetRequest, listener, now, ctx, fmt.Sprintf("Why%v", q)}
				case 2:
					sm = gotocol.Message{gotocol.Put, listener, now, ctx, fmt.Sprintf("Why%v %v", w, w*w)}
					w++ // put a new key each time
				}
				flow.AnnotateSend(sm, name,"NO") // service send logs creation time for this flow
				sm.GoSend(c)
			}
		}
	}
}
