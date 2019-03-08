package main
import (
	"github.com/gin-gonic/gin";
	// "flag"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"
	"strconv"
	"io/ioutil"
	"fmt"

	"github.com/adrianco/spigo/actors/edda"          // log configuration state
	"github.com/adrianco/spigo/tooling/archaius"     // store the config for global lookup
	"github.com/adrianco/spigo/tooling/architecture" // run an architecture from a json definition
	"github.com/adrianco/spigo/tooling/asgard"       // tools to create an architecture
	// "github.com/adrianco/spigo/tooling/collect"      // metrics to extvar
	"github.com/adrianco/spigo/tooling/flow"         // flow logging
	"github.com/adrianco/spigo/tooling/fsm"          // fsm and pirates
	"github.com/adrianco/spigo/tooling/gotocol"      // message protocol spec
	"github.com/adrianco/spigo/tooling/migration"    // migration from LAMP to netflixoss
	"github.com/adrianco/spigo/tooling/chaosmonkey"
	// "runtime/pprof"
)
var addrs string
var reload, graphmlEnabled, graphjsonEnabled, neo4jEnabled bool
var cpuprofile,confFile string 
var saveConfFile bool
var duration, cpucount int
var listener chan gotocol.Message
var noodles map[string]chan gotocol.Message
var eurekachan map[string]chan gotocol.Message
func test_func(c *gin.Context){
	for a := 0;a < 10;a ++{
		fmt.Println("I am runner ", "No.",a)
		time.Sleep(time.Second)
	}
}
func test_func_1() (string, int, int){
	return "abc",1,2
}
func HandlePost(C *gin.Context){
	body,_ := ioutil.ReadAll(C.Request.Body)
	fmt.Println(string(body))
}
func HandlePost1(C *gin.Context){
	id,_ :=strconv.ParseBool(C.DefaultQuery("id","false"))
	fmt.Println("id :",id)
	C.JSON(200,gin.H{
		"id":id,
		})
}
func pre_StartArch(c *gin.Context){
	archaius.Conf.Arch = c.DefaultQuery("a","netflixoss")
	archaius.Conf.Population,_ = strconv.Atoi(c.DefaultQuery("p","100"))
	duration,_ = strconv.Atoi(c.DefaultQuery("d","10"))
	archaius.Conf.Regions,_ = strconv.Atoi(c.DefaultQuery("w","1"))
	graphmlEnabled,_ = strconv.ParseBool(c.DefaultQuery("g","false"))
	graphjsonEnabled,_ = strconv.ParseBool(c.DefaultQuery("j","false"))
	neo4jEnabled,_ = strconv.ParseBool(c.DefaultQuery("n","false"))
	archaius.Conf.Msglog,_=strconv.ParseBool(c.DefaultQuery("m","false"))
	reload,_=strconv.ParseBool(c.DefaultQuery("r","false"))
	archaius.Conf.Collect,_=strconv.ParseBool(c.DefaultQuery("c","false"))
	addrs  = c.DefaultQuery("k","")
	archaius.Conf.StopStep,_  = strconv.Atoi(c.DefaultQuery("s","0"))
	archaius.Conf.EurekaPoll  = c.DefaultQuery("u","1s")
	archaius.Conf.Keyvals  = c.DefaultQuery("kv","")
	archaius.Conf.Filter,_  = strconv.ParseBool(c.DefaultQuery("f","false"))
	cpucount,_  = strconv.Atoi(c.DefaultQuery("cpus",string(runtime.NumCPU())))
	archaius.Conf.RunToEnd,_ =strconv.ParseBool(c.DefaultQuery("re","false"))
	// fmt.Println("Arch: ",Arch,"Population: ", Population,duration,Regions,graphmlEnabled,graphjsonEnabled)
	// fmt.Println(neo4jEnabled,Msglog,reload,Collect,addrs,StopStep,EurekaPoll,Keyvals,Filter,cpucount,RunToEnd)
	
	runtime.GOMAXPROCS(cpucount)
	var cpuprofile = c.DefaultQuery("cpuprofile","")
	var confFile = c.DefaultQuery("config","")
	var saveConfFile,_ = strconv.ParseBool(c.DefaultQuery("saveconfig","false"))
	kafkaAddrs := strings.Split(addrs,",")
	for _, addr := range kafkaAddrs {
		if len(addr) > 0 {
			archaius.Conf.Kafka = append(archaius.Conf.Kafka,addr)
		}
	}
	if confFile != ""{
		archaius.ReadConf(confFile)
	}
	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if graphjsonEnabled || graphmlEnabled || neo4jEnabled {
		if graphjsonEnabled {
			archaius.Conf.GraphjsonFile = archaius.Conf.Arch
		}
		if graphmlEnabled {
			archaius.Conf.GraphmlFile = archaius.Conf.Arch
		}
		if neo4jEnabled {
			if archaius.Conf.Filter {
				log.Fatal("Neo4j cannot be used with filtered names option -f")
			}
			pw := os.Getenv("NEO4JPASSWORD")
			url := os.Getenv("NEO4JURL")
			if pw == "" {
				log.Fatal("Neo4j requires environment variable NEO4JPASSWORD is set")
			}
			if url == "" {
				archaius.Conf.Neo4jURL = "localhost:7474"
			} else {
				archaius.Conf.Neo4jURL = url
			}
			log.Println("Graph will be written to Neo4j via NEO4JURL=" + archaius.Conf.Neo4jURL)
		}
		// make a big buffered channel so logging can start before edda is scheduled
		edda.Logchan = make(chan gotocol.Message, 1000)
	}
	archaius.Conf.RunDuration = time.Duration(duration) * time.Second
	// return
	
	if saveConfFile {
		archaius.WriteConf()
	}
	if archaius.Conf.Collect{
		f_flow,_ := os.Create("json_metrics/" + archaius.Conf.Arch + "_flow.json")
		f_flow.Close()//可否不在此处close()???
	}
	StartArch()//直接调用即可，多个http请求之间是可以异步处理的，所以没关系。
	c.JSON(200,gin.H{
		"Runtime":archaius.Conf.RunDuration ,
		"endless":archaius.Conf.RunToEnd,
	})
}
func StartArch(){
	// start up the selected architecture
	go edda.Start(archaius.Conf.Arch + ".edda") // start edda first
	if reload {
		var ServiceIndex int
		var ServiceNames map[int]string
		a := architecture.ReadArch(archaius.Conf.Arch)
		ServiceIndex,ServiceNames = architecture.ListNames(a)
		listener,noodles,eurekachan = architecture.Pre_Handle()
		asgard.Run(asgard.Reload(archaius.Conf.Arch), "","","","",ServiceNames,ServiceIndex)
	} else {
		switch archaius.Conf.Arch {
		case "fsm":
			fsm.Start()
		case "migration":
			migration.Start() // step by step from lamp to netflixoss
		default:
			a := architecture.ReadArch(archaius.Conf.Arch)
			if a == nil {
				log.Fatal("Architecture " + archaius.Conf.Arch + " isn't recognized")
			} else {
				if archaius.Conf.Population < 1 {
					log.Fatal("architecture: can't create less than 1 microservice")
				} else {
					log.Printf("architecture: scaling to %v%%", archaius.Conf.Population)
				}
				listener,noodles,eurekachan = architecture.Pre_Handle()
				fmt.Println(listener,noodles,eurekachan,"TY")
				log.Println(listener,noodles,eurekachan)
				architecture.Start(a)
			}
		}
	}
	log.Println("spigo: complete")
	// stop edda if it's running and wait for edda to flush messages
	if edda.Logchan != nil {
		close(edda.Logchan)
	}
	edda.Wg.Wait()
	if !archaius.Conf.RunToEnd{
		flow.Shutdown()
	}
	fmt.Println(listener,noodles,eurekachan,"TY")
	return
}
func EjectError(c *gin.Context){
	ErrorType := c.DefaultQuery("type","")
	Service1 := c.DefaultQuery("service1","")
	Service2 := c.DefaultQuery("service2","")
	DelayTime := c.DefaultQuery("dtime","")
	probability,_ := strconv.ParseFloat(c.DefaultQuery("pb","1.00"),32)
	if ErrorType == ""{
		c.JSON(200,gin.H{
			"ErrorCode":"Failed,need to set ErrorType",
		})
		return
	}
	if ErrorType == "Delete"{
		if Service1 != ""{
			chaosmonkey.Delete(&noodles,Service1)
			c.JSON(200,gin.H{
				"ErrorCode":0,
			})
			return
		}else{
			c.JSON(200,gin.H{
				"ErrorCode":"Need to set Delete Service",
			})
			return
		}
	}
	if ErrorType == "Delay"{
		if Service1 != "" && DelayTime != ""{
			chaosmonkey.Delay(&noodles,Service1,DelayTime)
			c.JSON(200,gin.H{
				"ErrorCode":0,
			})
			return
		}else{
			c.JSON(200,gin.H{
				"ErrorCode":"Need to set Delay Service or Delay Time",
			})
			return
		}
	}
	if ErrorType == "Disconnect"{
		if Service1 != "" && Service2 != ""{
			chaosmonkey.Disconnect(&noodles,Service1,DelayTime,float32(probability))
			c.JSON(200,gin.H{
				"ErrorCode":0,
			})
			return
		}else{
			c.JSON(200,gin.H{
				"ErrorCode":"Need to set Disconnect Service1 & 2",
			})
			return
		}
	}
	fmt.Println(listener,noodles,eurekachan,"Eject main")

	c.JSON(200,gin.H{
		"Result":"Success",
	})
}

func main(){
	r := gin.Default()
	// count := 0
	str,temp1,temp2 := test_func_1()
	fmt.Println(str,temp1,temp2)
    r.POST("/start", pre_StartArch)
    r.GET("/testSleep",test_func)
    r.POST("/eject",EjectError)
    r.Run() // listen and serve on 0.0.0.0:8080
}