// Package priamCassandra simulates a cassandra cluster with NetflixOSS Priam
// Takes incoming traffic and calls into cross zone and cross region nodes
package priamCassandra

import (
	"fmt"
	"github.com/chenhy/spigo/tooling/archaius"
	"github.com/chenhy/spigo/tooling/collect"
	"github.com/chenhy/spigo/tooling/flow"
	"github.com/chenhy/spigo/tooling/gotocol"
	"github.com/chenhy/spigo/tooling/handlers"
	"github.com/chenhy/spigo/tooling/names"
	"github.com/chenhy/spigo/tooling/ribbon"
	"hash/crc32"
	"sort"
	"strings"
	"time"
	"log"
)

// cassandra token to server map
type node struct {
	name  string
	token uint32
}

// ByToken ring of node names sorted by token
type ByToken []node

// implement node array sortable by Token interface
func (a ByToken) Len() int           { return len(a) }
func (a ByToken) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByToken) Less(i, j int) bool { return a[i].token < a[j].token }

// hash a string into the ring
func ringHash(s string) uint32 {
	return crc32.ChecksumIEEE([]byte(s))
}

// Find the node in the ring for a token
func (a ByToken) Find(h uint32) int {
	r := 0
	for i, n := range a {
		if n.token > h {
			break
		}
		r = i
	}
	return r
}

// Distribute tokens to one zone of a cassandra cluster, this doesn't yet allow for clusters to grow or replace nodes
func Distribute(cass map[string]chan gotocol.Message) string {
	size := len(cass)
	// each node owns a share of the full range
	hashrange := uint32(0xFFFFFFFF) / uint32(size)
	// make a config string of the form cass1:0,cass4:1000,cass2:2000
	i := 0
	s := ""
	for n := range cass {
		s += fmt.Sprintf("%s:%v,", n, hashrange*uint32(i))
		i++
	}
	s = strings.TrimSuffix(s, ",")
	// send the config to each node, repurposing the Chat message type as a kind of Gossip setup
	for _, c := range cass {
		gotocol.Send(c, gotocol.Message{gotocol.Chat, nil, time.Now(), gotocol.NilContext, s})
	}
	return s // for logging and test
}

// RingConfig gets the tokens for a ring
func RingConfig(m string) ByToken {
	s := strings.Split(m, ",")
	r := make(ByToken, len(s))
	for i, n := range s {
		nh := strings.Split(n, ":")
		if len(nh) == 2 {
			var h uint32
			fmt.Sscanf(nh[1], "%d", &h)
			r[i].name = nh[0]
			r[i].token = h
		}
	}
	sort.Sort(ByToken(r))
	return r
}

// Start priamCassandra, all configuration and state is sent via messages
func Start(listener chan gotocol.Message) {
	// remember the channel to talk to microservices
	microservices := ribbon.MakeRouter()
	// track the hash values owned by each node in the ring
	var ring ByToken
	dependencies := make(map[string]time.Time) // dependent services and time last updated
	store := make(map[string]string, 4)        // key value store
	store["why?"] = "because..."
	var parent chan gotocol.Message                                                                     // remember how to talk back to creator
	var name string                                                                                     // remember my name
	eureka := make(map[string]chan gotocol.Message, len(archaius.Conf.ZoneNames)*archaius.Conf.Regions) // service registry per zone and region
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
			if exit_symbol == 1{
				flow.Instrument(msg, name, hist, "DONE")
				flow.Add2Buffer(msg)
				continue
			}
			if msg.Imposition == gotocol.Put{
				flow.Instrument(msg, name, hist, "NO")
			}else if delaysymbol == 1 {
				log.Println("begin")
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
			case gotocol.NameDrop: // cross zone = true
				handlers.NameDrop(&dependencies, microservices, msg, name, listener, eureka, true)
			case gotocol.Forget:
				// forget a buddy
				handlers.Forget(&dependencies, microservices, msg)
			case gotocol.Chat:
				// Gossip setup notification of hash values for nodes, cass1:123,cass2:456
				ring = RingConfig(msg.Intention)
			case gotocol.GetRequest:
				// see if the data is stored on this node
				i := ring.Find(ringHash(msg.Intention))
				//log.Printf("%v: %v %v\n", name, i, ringHash(msg.Intention))
				if len(ring) == 0 || ring[i].name == name { // ring is setup so only respond if this is the right place
					// return any stored value for this key (Cassandra READ.ONE behavior)
					outmsg := gotocol.Message{gotocol.GetResponse, listener, time.Now(), msg.Ctx, store[msg.Intention]}
					flow.AnnotateSend(outmsg, name,"NO")
					outmsg.GoSend(msg.ResponseChan)
				} else {
					// forward the message to the right place, but don't change the ResponseChan or span
					outmsg := gotocol.Message{gotocol.GetRequest, msg.ResponseChan, time.Now(), msg.Ctx.AddSpan(), msg.Intention}
					flow.AnnotateSend(outmsg, name,"NO")
					outmsg.GoSend(microservices.Named(ring[i].name))
				}
			case gotocol.GetResponse:
				// return path from a request, send payload back up, not used by priamCassandra currently
			case gotocol.Put:
				// set a key value pair and replicate globally
				var key, value string
				fmt.Sscanf(msg.Intention, "%s%s", &key, &value)
				flow.Add2Buffer(msg)
				if key != "" && value != "" {
					i := ring.Find(ringHash(key))
					if len(ring) == 0 || ring[i].name == name { // ring is setup so only store if this is the right place
						store[key] = value
					} else {
						// forward the message to the right place, but don't change the ResponseChan or context parent
						outmsg := gotocol.Message{gotocol.Put, msg.ResponseChan, time.Now(), msg.Ctx.AddSpan(), msg.Intention}
						flow.AnnotateSend(outmsg, name,"NO")
						outmsg.GoSend(microservices.Named(ring[i].name))
					}
					// duplicate the request on to priamCassandra nodes in each zone and one in each region
					for _, z := range names.OtherZones(name, archaius.Conf.ZoneNames) {
						// replicate request
						for _, n := range microservices.Names() {
							if names.Region(n) == names.Region(name) && names.Zone(n) == z {
								outmsg := gotocol.Message{gotocol.Replicate, listener, time.Now(), msg.Ctx.NewParent(), msg.Intention}
								flow.AnnotateSend(outmsg, name,"NO")
								outmsg.GoSend(microservices.Named(n))
								break // only need to send it to one node in each zone
							}
						}
					}
					for _, r := range names.OtherRegions(name, archaius.Conf.RegionNames[0:archaius.Conf.Regions]) {
						for _, n := range microservices.Names() {
							if names.Region(n) == r {
								outmsg := gotocol.Message{gotocol.Replicate, listener, time.Now(), msg.Ctx.NewParent(), msg.Intention}
								flow.AnnotateSend(outmsg, name,"NO")
								outmsg.GoSend(microservices.Named(n))
								break // only need to send it to one node in each region
							}
						}
					}
				}
			case gotocol.Replicate:
				// Replicate is only used between priamCassandra nodes
				// end point for a request
				var key, value string
				fmt.Sscanf(msg.Intention, "%s%s", &key, &value)
				// log.Printf("priamCassandra: %v:%v", key, value)
				if key != "" && value != "" {
					i := ring.Find(ringHash(key))
					if len(ring) == 0 || ring[i].name == name { // ring is setup so only store if this is the right place
						store[key] = value
					} else {
						// forward the message to the right place, but don't change the ResponseChan
						outmsg := gotocol.Message{gotocol.Replicate, msg.ResponseChan, time.Now(), msg.Ctx, msg.Intention}
						flow.AnnotateSend(outmsg, name,"NO")
						outmsg.GoSend(microservices.Named(ring[i].name))
					}
				}
				// name looks like: netflixoss.us-east-1.zoneC.cassTurtle.priamCassandra.cassTurtle11
				myregion := names.Region(name)
				//log.Printf("%v: %v\n", name, myregion)
				// find if this was a cross region Replicate
				// find the name matching incoming request channel to see where its coming from
				in := microservices.NameChan(msg.ResponseChan)
				if in != "" && myregion != names.Region(in) {
					// Replicate from out of region needs to be Replicated once only to other zones in this Region
					for _, z := range names.OtherZones(name, archaius.Conf.ZoneNames) {
						// replicate request
						for _, n := range microservices.Names() {
							if names.Region(n) == myregion && names.Zone(n) == z {
								outmsg := gotocol.Message{gotocol.Replicate, listener, time.Now(), msg.Ctx.NewParent(), msg.Intention}
								flow.AnnotateSend(outmsg, name,"NO")
								outmsg.GoSend(microservices.Named(n))
								break // only need to send it to one node in each zone
							}
						}
					}
					break
				}
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
			case gotocol.Goodbye:
				gotocol.Message{gotocol.Final, nil, time.Now(), gotocol.NilContext, name}.GoSend(parent)
				flow.Add2Buffer(msg)
				exit_symbol = 1
				//return
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
		}
	}
}
