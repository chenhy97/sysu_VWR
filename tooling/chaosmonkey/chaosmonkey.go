// Package chaosmonkey deletes nodes
package chaosmonkey

import (
	"github.com/adrianco/spigo/tooling/gotocol"
	"github.com/adrianco/spigo/tooling/names"
	"log"
	"time"
	//"strconv"
)

// Delete a single node from the given service
func Delete(noodles *map[string]chan gotocol.Message, service string) {
	if service != "" {
		for node, ch := range *noodles {
			if names.Service(node) == service {//names.Service(node)返回的是服务类型
				gotocol.Message{gotocol.Goodbye, nil, time.Now(), gotocol.NewTrace(), "chaosmonkey"}.GoSend(ch)
				log.Println("chaosmonkey delete: " + node)
				log.Println("lslslslsls:"+names.Service(node)+"xsdsssdd:"+node+"xxxxx:")
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
