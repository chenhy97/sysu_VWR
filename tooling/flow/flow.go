// Package flow processes gotocol context information to collect and export request flows across the system
package flow

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	
	"github.com/adrianco/spigo/tooling/archaius"
	"github.com/adrianco/spigo/tooling/collect"
	"github.com/adrianco/spigo/tooling/dhcp"
	"github.com/adrianco/spigo/tooling/gotocol"
	"github.com/adrianco/spigo/tooling/graphneo4j"
	"github.com/go-kit/kit/metrics/generic"

)

// Values for zipkin span direction
type Values int

// Zipkin annotation tags
const (
	CS      Values = iota // client send
	SR                    // server receive
	SS                    // server send
	CR                    // client receive
	Unknown               // something went wrong
)

// pretty printer for Values
func (v Values) String() string {
	switch v {
	case CS:
		return "cs"
	case SR:
		return "sr"
	case SS:
		return "ss"
	case CR:
		return "cr"
	default:
		return "unknown"
	}
}

// flowmap is a map by traceid of slices of pointers to spannotations
type flowmaptype map[gotocol.TraceContextType][]*spannotype

// Annotation information for each step in the span
type spannotype struct {
	Ctx       string `json:"ctx"`        // Context as string
	Host      string `json:"host"`       // host name
	Imp       string `json:"imposition"` // protocol request type
	Intent    string `json:"intention"`  // request body
	Timestamp int64  `json:"ts"`         // unix nanotimestamp
	Value     string `json:"value"`      // direction of span
	Delay	  string `json:"tag_dlay"`
}

// ByCtx sortable spans
type ByCtx []*spannotype

func (a ByCtx) Len() int             { return len(a) }
func (a ByCtx) Swap(i, j int)        { a[i], a[j] = a[j], a[i] }
func (a ByCtx) Less(i, j int) bool { // sort by span first then time
	if a[i].Ctx == a[j].Ctx {
		return a[i].Timestamp < a[j].Timestamp
	}
	return a[i].Ctx < a[j].Ctx
}

var flowmap flowmaptype

var flowlock sync.Mutex // lock changes to the maps

// file to log flow data to
var file *os.File

var collector *KafkaCollector

var sync_flowmap sync.Map
var write_flowlock sync.RWMutex
// Common Annotation code
func annotate(msg gotocol.Message, name string, t time.Time, resp, others Values) *spannotype {
	if flowmap == nil {
		flowmap = make(flowmaptype, archaius.Conf.Population)

	}
	if flowmap[msg.Ctx.Trace] == nil {
		flowmap[msg.Ctx.Trace] = make([]*spannotype, 0, 2) // reserve space for at least 2 annotations in a span
	}
	annotation := new(spannotype)
	annotation.Host = name
	annotation.Ctx = msg.Ctx.String()
	annotation.Imp = msg.Imposition.String()
	annotation.Intent = msg.Intention
	annotation.Timestamp = t.UnixNano()

	if msg.Imposition == gotocol.GetResponse {
		annotation.Value = resp.String()
	} else {
		annotation.Value = others.String()
	}
	return annotation
}
func annotate_tag(msg gotocol.Message, name string, t time.Time, resp, others Values,tag_symbol string) *spannotype {
	if flowmap == nil {
		flowmap = make(flowmaptype, archaius.Conf.Population)

	}
	if flowmap[msg.Ctx.Trace] == nil {
		flowmap[msg.Ctx.Trace] = make([]*spannotype, 0, 2) // reserve space for at least 2 annotations in a span
	}
	annotation := new(spannotype)
	annotation.Host = name
	annotation.Ctx = msg.Ctx.String()
	annotation.Imp = msg.Imposition.String()
	annotation.Intent = msg.Intention
	annotation.Timestamp = t.UnixNano()

	annotation.Delay = tag_symbol//tag the request annotation

	if msg.Imposition == gotocol.GetResponse {
		annotation.Value = resp.String()
	} else {
		annotation.Value = others.String()
	}
	return annotation
}

// AnnotateReceive service activity when receiving a message
func AnnotateReceive(msg gotocol.Message, name string, received time.Time,tag_symbol string) {
	if !archaius.Conf.Collect {
		return
	}
	flowlock.Lock()
	if tag_symbol == "YES"{
		flowmap[msg.Ctx.Trace] = append(flowmap[msg.Ctx.Trace], annotate_tag(msg, name, received, CR, SR,tag_symbol))
	}else if tag_symbol == "NO"{
		flowmap[msg.Ctx.Trace] = append(flowmap[msg.Ctx.Trace], annotate(msg, name, received, CR, SR))
	}
	flowlock.Unlock()
	if graphneo4j.Enabled {
		trace := flowmap[msg.Ctx.Trace]
		if len(trace) >= 2 {
			graphneo4j.WriteFlow(strings.Replace(trace[len(trace)-2].Host, "-", "_", -1), strings.Replace(trace[len(trace)-1].Host, "-", "_", -1), trace[1].Imp, trace[1].Timestamp, msg.Ctx.Trace)
		}
	}
	return
}
// AnnotateSend service sends on a flow
func AnnotateSend(msg gotocol.Message, name string,tag_symbol string) {
	if !archaius.Conf.Collect {
		return
	}
	flowlock.Lock()
	if tag_symbol == "YES"{
		flowmap[msg.Ctx.Trace] = append(flowmap[msg.Ctx.Trace], annotate_tag(msg, name, msg.Sent, SS, CS,tag_symbol))

	}else if tag_symbol == "NO"{
		flowmap[msg.Ctx.Trace] = append(flowmap[msg.Ctx.Trace], annotate(msg, name, msg.Sent, SS, CS))
	}
	//log.Println(flowmap[msg.Ctx.Trace])
	//log.Println("^^^^^^^^^^^^^^")
	flowlock.Unlock()
	return
}
// End a flow, flushing output and freeing the request id to keep the map smaller
func End(msg gotocol.Message, resphist, servhist, rthist *generic.Histogram) {
	if !archaius.Conf.Collect {
		return
	}
	var cs, sr, ss, cr int64
	// find the annotations for the client send time and the client receive time
	for _, a := range flowmap[msg.Ctx.Trace] {
		if a.Value == CS.String() {
			cs = a.Timestamp
		}
		if a.Value == SR.String() {
			sr = a.Timestamp
		}
		if a.Value == SS.String() {
			ss = a.Timestamp
		}
		if a.Value == CR.String() {
			cr = a.Timestamp
		}
	}
	if cs > 0 && cr > 0 { // response time measured at the client
		collect.Measure(resphist, time.Unix(0, cr).Sub(time.Unix(0, cs)))
	}
	if ss > 0 && sr > 0 { // service time measured at the server
		collect.Measure(servhist, time.Unix(0, ss).Sub(time.Unix(0, sr)))
	}
	if cs > 0 && cr > 0 && ss > 0 && sr > 0 { // network time sum of each direction
		collect.Measure(rthist, time.Unix(0, sr).Sub(time.Unix(0, cs))+time.Unix(0, cr).Sub(time.Unix(0, ss)))
	}
}
func Add2Buffer(msg gotocol.Message){
	write_flowlock.RLock()
	sync_flowmap.Store(msg.Ctx.Trace,flowmap[msg.Ctx.Trace])
	write_flowlock.RUnlock()
}
// Shutdown the flow mapping system and flush remaining flows
func Shutdown() {
	if !archaius.Conf.Collect {
		return
	}

	// Try to add Kafka collector if configured to do so
	if len(archaius.Conf.Kafka) > 0 {
		log.Printf("Flushing flows to Kafka Collector %#v\n", archaius.Conf.Kafka)
		var err error
		collector, err = NewKafkaCollector(archaius.Conf.Kafka)
		if err != nil {
			log.Printf("Unable to start Kafka Collector: %#v\n", err)
		} else {
			defer collector.Close()
		}
	}

	flowlock.Lock()
	defer flowlock.Unlock()
	f, err := os.Create("json_metrics/" + archaius.Conf.Arch + "_flow.json")
	if err != nil {
		log.Fatal(err)
	} else {
		file = f
	}
	log.Printf("Flushing flows to %v\n", file.Name())
	file.WriteString("[\n")
	comma := false
	for c, f := range flowmap {
		if comma {
			file.WriteString(",\n")
		} else {
			comma = true
		}
		Flush(c, f)
	}
	file.WriteString("\n]\n")
	file.Close()
	flowmap = nil
	gotocol.ClearTrace()
	return
}
//kafka arch don't set,so we can't collect flow metrics during running
//这种方法的bug,当出现Delete类型的错误时，无法抓到路径上经过Delete节点的所有trace
//
func Interval_save(){
	if !archaius.Conf.RunToEnd {
		return
	}
	store_ticker := time.NewTicker(time.Second*1/4)
	for _ = range store_ticker.C{
		func(){
			fi,err := os.OpenFile("json_metrics/" + archaius.Conf.Arch + "_flow.json", os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				log.Fatal(err)
			} else {
				file = fi
			}
			write_flowlock.Lock()
			sync_flowmap.Range(func(k,v interface{})bool {
				c,ok1 := k.(gotocol.TraceContextType)
				f,ok2 := v.([]*spannotype)
				if ok1 && ok2 {
					Flush(c, f)
					sync_flowmap.Delete(k)
				}else{
					fmt.Println("Wrong insertion!!!!")
				}
				file.WriteString(",\n")
				return true
			})
			write_flowlock.Unlock()
			file.Close()
		}()
	}
}
/* example: Zipkin format is an array of these
{
  "traceId": "5e27c67030932221",
  "name": "GET",
  "id": "38357d8f309b379d",
  "parentId": "5e27c67030932221",
  "annotations": [
    {
      "endpoint": {
        "serviceName": "zipkin-query",
        "ipv4": "127.0.0.1"
      },
      "timestamp": 1444780030334000,
      "value": "cs"
    },
    {
      "endpoint": {
        "serviceName": "zipkin-query",
        "ipv4": "172.17.0.84"
      },
      "timestamp": 1444780030643000,
      "value": "sr"
    },
    {
      "endpoint": {
        "serviceName": "zipkin-query",
        "ipv4": "127.0.0.1"
      },
      "timestamp": 1444780031521000,
      "value": "cr"
    },
    {
      "endpoint": {
        "serviceName": "zipkin-query",
        "ipv4": "172.17.0.84"
      },
      "timestamp": 1444780031689000,
      "value": "ss"
    }
  ]
},
*/

// endpoint for zipkin
type zipkinendpoint struct {
	Servicename string `json:"serviceName"`
	Ipv4        string `json:"ipv4"`
	Port        int    `json:"port"`
}

// annotation for zipkin
type zipkinannotation struct {
	Endpoint  zipkinendpoint `json:"endpoint"`
	Timestamp int64          `json:"timestamp"`
	Value     string         `json:"value"`
	Intent_code string       `json:"intent_code"`
	Delay 	  string         `json:"tag_dlay"`
}

// trace for zipkin
type zipkinspan struct {
	Traceid     string             `json:"traceId"`
	Name        string             `json:"name"`
	Id          string             `json:"id"`
	ParentId    string             `json:"parentId,omitempty"`
	Annotations []zipkinannotation `json:"annotations"`
}

// WriteZip stores zipkin as json
func WriteZip(zip zipkinspan) {
	j, err := json.Marshal([]*zipkinspan{&zip})
	// print(j,"try")
	if err != nil {
		log.Fatal(err)
	}
	if collector != nil {
		collector.Collect(j)
	}
	file.Write(j[1 : len(j)-1])
}

// Flush the spans for a request in zipkin format
func Flush(t gotocol.TraceContextType, trace []*spannotype) {
	var zip zipkinspan
	var ctx string
	n := -1
	sort.Sort(ByCtx(trace))
	for _, a := range trace {
		if ctx != a.Ctx { // new span
			if ctx != "" { // not the first
				WriteZip(zip)
				file.WriteString(",\n")
				zip.Annotations = nil
			}
			n++
			zip.Traceid = fmt.Sprintf("%016d", t) // pad id's to 16 characters to keep zipkin happy
			zip.Name = a.Imp
			s := strings.SplitAfter(a.Ctx, "s")                            // tXpYsZ -> [tXpYs, Z]
			p := strings.TrimSuffix(strings.SplitAfter(s[0], "p")[1], "s") // tXpYs -> [tXp, Ys] -> Ys -> Y
			zip.Id = "000000000000000"[0:(16-len(s[1]))] + s[1]            // pad id's to 16 characters to keep zipkin happy
			if p != "0" {
				zip.ParentId = "000000000000000"[0:(16-len(p))] + p // pad id's to 16 characters to keep zipkin happy
			}
			ctx = a.Ctx
		}
		var ann zipkinannotation
		ann.Endpoint.Servicename = a.Host
		ann.Endpoint.Ipv4 = dhcp.Lookup(a.Host)
		ann.Endpoint.Port = 8080
		ann.Timestamp = a.Timestamp / 1000 // convert from UnixNano to Microseconds
		ann.Value = a.Value
		ann.Intent_code = a.Intent
		ann.Delay = a.Delay
		// fmt.Println(a.delay,"hi")
		// if(ann.Delay == "YES"){
		// 	fmt.Println(ann)
		// }
		zip.Annotations = append(zip.Annotations, ann)
	}
	WriteZip(zip)
}

// Instrument common code for requests
func Instrument(msg gotocol.Message, name string, hist *generic.Histogram,tag_symbol string) {
	received := time.Now()//感觉这里可以有点东西，加个随机值
	collect.Measure(hist, received.Sub(msg.Sent))
	if archaius.Conf.Msglog {
		log.Printf("%v: %v +++++====\n", name, msg)
	}
	if msg.Ctx != gotocol.NilContext {
		AnnotateReceive(msg, name, received,tag_symbol) // store the annotation for this request
		//tag_symbol in order to tag the requeset since delay
	}
}
