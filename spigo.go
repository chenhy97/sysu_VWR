package main

import (
	"github.com/gin-gonic/gin";
	"fmt"
	// "flag"
	"io"
	// "net/http"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"
	"net/http"
    "regexp"
    // "time"
	// "github.com/gin-contrib/cors"
	"github.com/adrianco/spigo/actors/edda"          // log configuration state
	"github.com/adrianco/spigo/tooling/archaius"     // store the config for global lookup
	"github.com/adrianco/spigo/tooling/architecture" // run an architecture from a json definition
	"github.com/adrianco/spigo/tooling/asgard"       // tools to create an architecture
	"github.com/adrianco/spigo/tooling/chaosmonkey"
	// "github.com/adrianco/spigo/tooling/collect"      // metrics to extvar
	"github.com/adrianco/spigo/tooling/flow"      // flow logging
	"github.com/adrianco/spigo/tooling/fsm"       // fsm and pirates
	"github.com/adrianco/spigo/tooling/gotocol"   // message protocol spec
	"github.com/adrianco/spigo/tooling/migration" // migration from LAMP to netflixoss
	// "runtime/pprof"
	"github.com/adrianco/go-vizceral/arch2vizceral"
)
var addrs string
var reload, graphmlEnabled, graphjsonEnabled, neo4jEnabled bool
var cpuprofile,confFile string 
var saveConfFile bool
var duration, cpucount int
var listener *chan gotocol.Message
var noodles *map[string]chan gotocol.Message
var eurekachan *map[string]chan gotocol.Message
var user_name string
//调用os.MkdirAll递归创建文件夹
func createDir(filePath string)  error  {
	if !isExist(filePath) {
		err := os.MkdirAll(filePath,os.ModePerm)
		return err
	}
	return nil
}
// 判断所给路径文件/文件夹是否存在(返回true是存在)
func isExist(path string) bool {
	_, err := os.Stat(path)    //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}
func upload_file(c *gin.Context){
	name := c.PostForm("a")
	user_name = c.PostForm("u")
	RunToEnd := c.PostForm("re")
	fmt.Println(name,user_name,RunToEnd)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		fmt.Println("error")
		c.JSON(404,gin.H{
			"ErrorCode":"Set File",
		})
		return
	}
	filename := header.Filename
	fmt.Println("json_arch/"+user_name,filename)
	createDir("json_arch/" + user_name)
	out, err := os.Create("json_arch/" + user_name +"/"  +name+"_arch.json")
	defer out.Close()
	io.Copy(out, file)
	c.JSON(200,gin.H{
			"ErrorCode":"Failed,need to set ErrorType",
		})
	return
}
func pre_StartArch(c *gin.Context){
	timeStr := time.Now().Format("2006-01-02 15:04:05")
	user_name = c.PostForm("un")
	createDir("json_arch/"+user_name)
	createDir("json_metrics/"+user_name)
	createDir("gml/"+user_name)
	createDir("json/"+user_name)
	createDir("csv_metrics/"+user_name)
	inputfile_name := c.DefaultPostForm("a","netflixoss")
	archaius.Conf.Arch = user_name + "/" + inputfile_name + timeStr
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		fmt.Println("error")
		c.JSON(404,gin.H{
			"ErrorCode":"Set File",
		})
		return
	}
	filename := header.Filename
	fmt.Println("json_arch/"+user_name,filename)
	createDir("json_arch/" + user_name)
	out, err := os.Create("json_arch/" + user_name +"/"  +inputfile_name + timeStr +"_arch.json")
	io.Copy(out, file)
	out.Close()

	str := arch2vizceral.A2v(archaius.Conf.Arch)
	archaius.Conf.Population,_ = strconv.Atoi(c.DefaultPostForm("p","100"))
	fmt.Println(archaius.Conf.Population,c.DefaultPostForm("p","100"))
	duration,_ = strconv.Atoi(c.DefaultPostForm("d","10"))
	archaius.Conf.Regions,_ = strconv.Atoi(c.DefaultPostForm("w","1"))
	graphmlEnabled,_ = strconv.ParseBool(c.DefaultPostForm("g","false"))
	graphjsonEnabled,_ = strconv.ParseBool(c.DefaultPostForm("j","false"))
	neo4jEnabled,_ = strconv.ParseBool(c.DefaultPostForm("n","false"))
	archaius.Conf.Msglog,_=strconv.ParseBool(c.DefaultPostForm("m","false"))
	reload,_=strconv.ParseBool(c.DefaultPostForm("r","false"))
	archaius.Conf.Collect,_=strconv.ParseBool(c.DefaultPostForm("c","false"))
	addrs  = c.DefaultPostForm("k","")
	archaius.Conf.StopStep,_  = strconv.Atoi(c.DefaultPostForm("s","0"))
	archaius.Conf.EurekaPoll  = c.DefaultPostForm("u","1s")
	fmt.Println(archaius.Conf.EurekaPoll)
	archaius.Conf.Keyvals  = c.DefaultPostForm("kv","")
	archaius.Conf.Filter,_  = strconv.ParseBool(c.DefaultPostForm("f","false"))
	cpucount = runtime.NumCPU()
	archaius.Conf.RunToEnd,_ =strconv.ParseBool(c.DefaultPostForm("re","false"))
	// fmt.Println("Arch: ",Arch,"Population: ", Population,duration,Regions,graphmlEnabled,graphjsonEnabled)
	// fmt.Println(neo4jEnabled,Msglog,reload,Collect,addrs,StopStep,EurekaPoll,Keyvals,Filter,cpucount,RunToEnd)
	
	runtime.GOMAXPROCS(cpucount)
	var cpuprofile = c.DefaultPostForm("cpuprofile","")
	var confFile = c.DefaultPostForm("config","")
	var saveConfFile,_ = strconv.ParseBool(c.DefaultPostForm("saveconfig","false"))
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
	c.JSON(200,gin.H{
		"Runtime":archaius.Conf.RunDuration ,
		"endless":archaius.Conf.RunToEnd,
		"elsatic_search":"json_arch/" + user_name +"/"  +inputfile_name + timeStr +"_arch.json",
		"vizceral_file":str,
	})
	StartArch()//直接调用即可，多个http请求之间是可以异步处理的，所以没关系。
	// fmt.Println("Success!")
	
	// fmt.Println(str)
	
	// return
}
func StartArch(){
	// start up the selected architecture
	go edda.Start(archaius.Conf.Arch + ".edda") // start edda first
	if reload {
		var ServiceIndex int
		var ServiceNames map[int]string
		a := architecture.ReadArch(archaius.Conf.Arch,true)
		ServiceIndex,ServiceNames = architecture.ListNames(a)
		listener,noodles,eurekachan = architecture.Pre_Handle()
		asgard.Run(asgard.Reload(archaius.Conf.Arch),ServiceNames,ServiceIndex)
	} else {
		switch archaius.Conf.Arch {
		case "fsm":
			fsm.Start()
		case "migration":
			migration.Start() // step by step from lamp to netflixoss
		default:
			a := architecture.ReadArch(archaius.Conf.Arch,true)
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
				architecture.Start(noodles,a)
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
	fmt.Println(listener,*noodles,eurekachan,"TY")
	return
}
func ejectError(c *gin.Context){
	ErrorType := c.DefaultPostForm("type","")
	Service1 := c.DefaultPostForm("service1","")
	Service2 := c.DefaultPostForm("service2","")
	DelayTime := c.DefaultPostForm("dtime","")
	probability,_ := strconv.ParseFloat(c.DefaultPostForm("pb","1.00"),32)
	fmt.Println(ErrorType,Service1,Service2,DelayTime,probability)
	// return
	if ErrorType == ""{
		c.JSON(404,gin.H{
			"ErrorCode":"Failed,need to set ErrorType",
		})
		return
	}
	if ErrorType == "Delete"{
		if Service1 != ""{
			chaosmonkey.Delete(noodles,Service1)
			c.JSON(200,gin.H{
				"ErrorCode":0,
			})
			return
		}else{
			c.JSON(404,gin.H{
				"ErrorCode":"Need to set Delete Service",
			})
			return
		}
	}
	if ErrorType == "Delay"{
		if Service1 != "" && DelayTime != ""{
			DelayTime = DelayTime + "ms"
			chaosmonkey.Delay(noodles,Service1,DelayTime)
			c.JSON(200,gin.H{
				"ErrorCode":0,
			})
			return
		}else{
			c.JSON(404,gin.H{
				"ErrorCode":"Need to set Delay Service or Delay Time",
			})
			return
		}
	}
	if ErrorType == "Disconnect"{
		if Service1 != "" && Service2 != ""{
			chaosmonkey.Disconnect(noodles,Service1,Service2,float32(probability))
			c.JSON(200,gin.H{
				"ErrorCode":0,
			})
			return
		}else{
			c.JSON(404,gin.H{
				"ErrorCode":"Need to set Disconnect Service1 & 2",
			})
			return
		}
	}
	fmt.Println(listener,*noodles,eurekachan,"Eject main")

	c.JSON(200,gin.H{
		"Result":"Success",
	})
}
func exit(c *gin.Context){
	asgard.Exit()
}
func test(c *gin.Context){
	arch2vizceral.A2v("test")
	check := c.DefaultPostForm("check_data","winner")
	time :=c.DefaultPostForm("time","~")
	fmt.Println(check)
	fmt.Println("YES")
	c.JSON(200,gin.H{
		"return":"Success",
		"check_data":check,
		"time":time,
		})
}
func main(){
	//fmt.Println(arch2vizceral.A2v("test"))
	r := gin.Default()
	r.Use(CorsMiddleware())
	// count := 0
    // r.POST("/start", pre_StartArch)

    r.POST("/start",pre_StartArch)
    r.POST("/eject",ejectError)
    r.POST("/test",test)
    r.POST("/exit",exit)
    r.Run(":9000") // listen and serve on 0.0.0.0:8080
}
func CorsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        method := c.Request.Method
        origin := c.Request.Header.Get("Origin")
        fmt.Println(origin)
        var filterHost = [...]string{"http://0.0.0.0:8000"}
        // filterHost 做过滤器，防止不合法的域名访问
        var isAccess = false
        for _, v := range(filterHost) {
            match, _ := regexp.MatchString(v, origin)
            if match {
                isAccess = true
            }
        }
        if isAccess {
        // 核心处理方式
            c.Header("Access-Control-Allow-Origin", "*")
            c.Header("Access-Control-Allow-Headers", "access-control-allow-headers,access-control-allow-methods,access-control-allow-origin")
            c.Header("Access-Control-Allow-Methods", "GET, OPTIONS, POST, PUT, DELETE")
            c.Set("content-type", "application/json")
        }
        //放行所有OPTIONS方法
        if method == "OPTIONS" {
            // c.JSON(http.StatusOK, "Options Request!")
        	c.AbortWithStatus(http.StatusNoContent)
        }
      	c.Next()
    }
 }
