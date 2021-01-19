package main

import (
	"container/list"
	"encoding/json"
	"github.com/go-ini/ini"
	"github.com/kirinlabs/HttpRequest"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	loginToken  string
	domain      string
	userAgent   string
	oldIp       string
	minute      time.Duration
	currentPath string
)

func init() {
	ePath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	currentPath = filepath.Dir(ePath)
	logFile, err := os.OpenFile(filepath.Join(currentPath, "dns.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal("打开日志文件失败:", err)
	}
	writers := []io.Writer{
		logFile,
		os.Stdout}
	//同时写文件和控制台
	fileAndStdoutWriter := io.MultiWriter(writers...)
	log.SetOutput(fileAndStdoutWriter)
	log.SetFlags(log.LstdFlags)
}

func main() {
	var mapList = list.New()
	cfg, err := ini.Load(filepath.Join(currentPath, "dns.ini"))
	if err != nil {
		log.Fatal("加载配置文件失败: ", err)
	}
	log.Print("开始ddns服务...")
	for _, s := range cfg.Sections() {
		switch s.Name() {
		case "DEFAULT":
			break
		case "common":
			loginToken = s.Key("login_token").String()
			domain = s.Key("domain").String()
			userAgent = s.Key("user_agent").String()
			var minuteInt int64
			minuteInt, err = s.Key("minute").Int64()
			if err != nil || minuteInt < 2 {
				minuteInt = 1
			}
			minute = time.Duration(minuteInt * 60 * 1000 * 1000 * 1000)
			break
		default:
			var subDomain = s.Key("sub_domain").String()
			var recordType = s.Key("record_type").String()
			var recordLine = s.Key("record_line").String()
			var ttl = s.Key("ttl").String()
			mapList.PushBack(subDomain + "," + recordType + "," + recordLine + "," + ttl)
		}
	}
	log.Print(minute)
	for {
		execute(mapList)
		time.Sleep(minute)
	}
}

func execute(mapList *list.List) {
	var ipv6 = getMyIPV6()
	if ipv6 == "" {
		log.Print("未获取到ipv6")
		return
	}
	if ipv6 != oldIp {
		oldIp = ipv6
		for e := mapList.Front(); e != nil; e = e.Next() {
			split := strings.Split(e.Value.(string), ",")
			var subDomain = split[0]
			var recordType = split[1]
			var recordLine = split[2]
			var ttl = split[3]
			var m = make(map[string]interface{})
			m["sub_domain"] = subDomain
			m["record_type"] = recordType
			result := post("Record.List", m)
			records := result["records"].([]interface{})
			if len(records) == 0 {
				m["record_line"] = recordLine
				m["ttl"] = ttl
				m["value"] = ipv6
				post("Record.Create", m)
				log.Print(subDomain + "." + domain + " ->> " + ipv6)
			} else {
				record := records[0]
				recordMap := record.(map[string]interface{})
				id := recordMap["id"].(string)
				value := recordMap["value"].(string)
				//ip变动了 更新数据
				if value != ipv6 {
					m["record_id"] = id
					m["record_line"] = recordLine
					m["ttl"] = ttl
					m["value"] = ipv6
					post("Record.Modify", m)
					log.Print(subDomain + "." + domain + " ->> " + ipv6)
				}
			}
		}
	} else {
		log.Print("ip没有变化")
	}
}

/**
获取本机ipV6
*/
func getMyIPV6() string {
	s, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range s {
		i := regexp.MustCompile(`(\w+:){7}\w+`).FindString(a.String())
		if strings.Count(i, ":") == 7 {
			return i
		}
	}
	return ""
}

/**
json转map
*/
func jsonToMap(j string) map[string]interface{} {
	m := make(map[string]interface{})
	err := json.Unmarshal([]byte(j), &m)
	if err != nil {
		log.Println(err, "不是json")
	}
	return m
}

/*
map转json
*/
func mapToJson(m map[string]interface{}) string {
	str, _ := json.Marshal(m)
	return string(str)
}

func post(url string, data map[string]interface{}) map[string]interface{} {
	data["login_token"] = loginToken
	data["format"] = "json"
	data["lang"] = "cn"
	data["error_on_empty"] = "no"
	data["domain"] = domain
	req := HttpRequest.NewRequest()
	req.SetHeaders(map[string]string{
		"Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
		"UserAgent":    userAgent,
	})
	//自动将Map以Json方式发送参数
	resp, err := req.Post("https://dnsapi.cn/"+url, data)
	if err != nil {
		log.Print(err, 1)
	}
	body, err := resp.Body()
	return jsonToMap(string(body))
}
