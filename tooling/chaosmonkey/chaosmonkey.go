// Package chaosmonkey deletes nodes
package chaosmonkey

import (
	"github.com/chenhy/spigo/tooling/gotocol"
	"github.com/chenhy/spigo/tooling/names"
	"log"
	"time"
	"math/rand"
	//"strconv"
)

// Delete a single node from the given service
func Delete(noodles *map[string]chan gotocol.Message, service string) {
	if service != "" {
		for node, ch := range *noodles {
			if names.Service(node) == service {//names.Service(node)返回的是服务类型
				gotocol.Message{gotocol.Goodbye, nil, time.Now(), gotocol.NewTrace(), "chaosmonkey"}.GoSend(ch)
				return
			}
		}
	}
}
func Delay(noodles *map[string]chan gotocol.Message, service string, dtime string) {
	if service != ""{
		for node, ch := range *noodles {
			if names.Service(node) == service {
				gotocol.Message{gotocol.Delay, nil, time.Now(),gotocol.NewTrace(), dtime}.GoSend(ch)
				log.Println("delaymonkey delay: " + node + " for " + dtime)
				return
			}
		}
	}
}
func Disconnect(noodles *map[string]chan gotocol.Message, serviceA string, serviceB string, probablity float32){
	seed := rand.NewSource(time.Now().Unix())
	r := rand.New(seed)
	if serviceA != "" && serviceB != ""{
		for nodeA, chA := range *noodles {
			if names.Service(nodeA) == serviceA {
				for nodeB := range *noodles{
					if names.Service(nodeB) == serviceB{
						random := r.Float32()
						// log.Println
						if random < probablity{
							gotocol.Message{gotocol.Forget,nil,time.Now(),gotocol.NewTrace(),nodeB}.GoSend(chA)
							log.Println("Disconnectmonkey disconnect : " + nodeA + " " + nodeB)
						}
					}
				}
			}
		}
	}
}