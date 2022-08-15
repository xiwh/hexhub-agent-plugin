package plugin

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/wonderivan/logger"
	"io"
	"net"
	"net/http"
	"os"
)

var apiEndpoint = "https://api.gaydev.cc"
var mainPluginEndpoint string
var mToken string
var mThisEndpoint string
var mPluginId string

type pluginHandler struct {
}

func (t *pluginHandler) ServeHTTP(write http.ResponseWriter, req *http.Request) {
	token := req.Header.Get("Token")
	if token != mToken {
		write.WriteHeader(500)
	}
}

func Start(pluginId string) {
	mPluginId = pluginId
	mainPluginEndpoint = *flag.String("address", "http://127.0.0.1:35580", "")
	mToken = *flag.String("token", "", "")
	if mToken == "" {
		panic("mToken is empty")
	}
	tcpAddress, err := net.ResolveTCPAddr("tcp", "0.0.0.0:0")
	if err != nil {
		logger.Error(err)
		panic(err)
		return
	}
	err = http.ListenAndServe(fmt.Sprintf("%s:%d", tcpAddress.IP.String(), tcpAddress.Port), new(pluginHandler))
	if err != nil {
		logger.Error(err)
		panic(err)
		return
	}
	registerPlugin()
	http.HandleFunc("kill", func(writer http.ResponseWriter, request *http.Request) {
		os.Exit(0)
	})
}

func registerPlugin() {
	err := Post("register-plugin", nil, nil)
	if err != nil {
		logger.Error(err)
		panic(err)
	}
}

func Post(uri string, req any, result any) error {
	var reqBuf *bytes.Buffer
	if req != nil {
		reqData, err := json.Marshal(req)
		if err != nil {
			return err
		}
		reqBuf = bytes.NewBuffer(reqData)
	}

	client := &http.Client{}
	//生成要访问的url
	url := fmt.Sprintf("%s/%s", mainPluginEndpoint, uri)

	//提交请求
	reqest, err := http.NewRequest("POST", url, reqBuf)

	//增加header选项
	reqest.Header.Add("Token", mToken)
	reqest.Header.Add("PluginId", mPluginId)
	reqest.Header.Add("Accept", "application/json")
	reqest.Header.Add("Content-Type", "application/json")

	if err != nil {
		return err
	}
	//处理返回结果
	response, _ := client.Do(reqest)
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Error(err)
		}
	}(response.Body)
	if result == nil {
		return nil
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, result)
	return err

}

func GetMainPluginEndpoint() string {
	return mainPluginEndpoint
}

func GetMyEndpoint() string {
	return mThisEndpoint
}

func GetApiEndpoint() string {
	return apiEndpoint
}
