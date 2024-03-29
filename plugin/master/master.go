package master

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/vulcand/oxy/forward"
	"github.com/wonderivan/logger"
	"github.com/xiwh/hexhub-agent-plugin/plugin"
	httputil2 "github.com/xiwh/hexhub-agent-plugin/util/httputil"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var CurrentVersion int
var CurrentVersionName string
var mForward *forward.Forwarder
var mAllowedDomainNames = cmap.New[any]()

// AutoExitTimeLimit 自动退出时限,超过多久没有进行请求交互才自动退出
const AutoExitTimeLimit = 5 * 60000

const MasterId = "master"

const PluginStatusNotStarted = 0
const PluginStatusStarting = 1
const PluginStatusRunning = 2
const PluginStatusDownloading = 3
const PluginStatusDownloadFailed = 4
const PluginStatusInstallFailed = 5

var RouteMap = make(map[string]func(http.ResponseWriter, *http.Request))

type PluginInfo struct {
	Id             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Version        int    `json:"version"`
	VersionName    string `json:"versionName"`
	ExecEnter      string `json:"execEnter"`
	Status         int    `json:"status"`
	TotalSize      int64  `json:"totalSize"`
	DownloadedSize int64  `json:"downloadedSize"`
	ErrorMsg       string `json:"errorMsg"`
	Endpoint       string `json:"endpoint"`
	PluginDir      string `json:"pluginDir"`
	AutoExit       bool   `json:"autoExit"`
	Connections    int64  `json:"connections"`
	LastConnTime   int64  `json:"lastConnTime"`
	lock           *sync.Mutex
	cmd            *exec.Cmd
}

type masterInfo struct {
	Namespace   string `json:"namespace"`
	Version     int    `json:"version"`
	VersionName string `json:"versionName"`
}

type masterHttpHandle struct {
}

func Start(namespace string, version int, versionName, apiEndpoint string, allowedDomainNames string, port int, debug bool) {
	CurrentVersion = version
	CurrentVersionName = versionName
	mAllowedDomainNames.Clear()
	domainNameStrArr := strings.Split(allowedDomainNames, ",")
	for _, s := range domainNameStrArr {
		mAllowedDomainNames.Set(s, nil)
	}
	token := uuid.New().String()
	plugin.Init(MasterId, namespace, apiEndpoint, port, debug, token)
	manifests, err := plugin.GetManifests()
	if err != nil {
		panic(err)
	}
	for _, manifest := range manifests {
		initManifest(manifest)
	}
	// Forwards incoming requests to whatever location URL points to, adds proper forwarding headers
	mForward, _ = forward.New()
	heartbeat()
	pluginCoDeath()
	panic(http.ListenAndServe(plugin.AgentAddr, new(masterHttpHandle)))
}

func RegisterRoute(pattern string, f func(http.ResponseWriter, *http.Request)) {
	RouteMap[strings.TrimLeft(pattern, "/")] = f
}

func (t masterHttpHandle) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	refererUrl, _ := url.Parse(req.Header.Get("Referer"))
	originUrl, _ := url.Parse(req.Header.Get("Origin"))
	//判断是否允许访问
	if req.Header.Get("Token") != "" {
		//如果有token则验证token是否有效
		if !checkToken(req) {
			writer.WriteHeader(401)
			writer.Write([]byte("<h1>session has expired</h1>"))
			return
		}
	} else {
		//否则验证origin或referer域名是否有效
		if !mAllowedDomainNames.Has(originUrl.Host) && !mAllowedDomainNames.Has(refererUrl.Host) {
			writer.WriteHeader(401)
			writer.Write([]byte("<h1>session invalid</h1>"))
			return
		}
	}

	//允许跨域处理
	header := writer.Header()
	header.Add("Access-Control-Allow-Origin", httputil2.GetSchemeHost(originUrl))
	header.Add("Access-Control-Allow-Credentials", "true")
	header.Add("Access-Control-Allow-Methods", "GET, POST, HEAD, PATCH, PUT, DELETE, OPTIONS")
	header.Add("Access-Control-Allow-Headers", "*")
	header.Add("Access-Control-Expose-Headers", "*")
	if req.Method == "OPTIONS" {
		writer.WriteHeader(200)
		return
	}

	uri := req.URL.Path
	switch uri {
	case "":
	case "/":
	case "/ping":
		pingHandler(writer, req)
		break
	case "/info":
		infoHandler(writer, req)
		break
	case "/plugin/list":
		pluginListHandler(writer, req)
		break
	case "/plugin/info":
		pluginInfoHandler(writer, req)
		break
	case "/plugin/start":
		pluginStartHandler(writer, req)
		break
	case "/plugin/restart":
		pluginRestartHandler(writer, req)
		break
	case "/plugin/stop":
		pluginStopHandler(writer, req)
		break
	case "/plugin/uninstall":
		pluginUninstallHandler(writer, req)
		break
	case "/plugin/check-update":
		pluginCheckUpdateHandler(writer, req)
		break
	case "/plugin/register":
		pluginRegisterHandler(writer, req)
		break
	default:
		uri = strings.TrimLeft(uri, "/")
		arr := strings.Split(uri, "/")
		if arr[0] == "master" {
			uri = strings.Join(arr[1:], "/")
			handFunc := RouteMap[uri]
			if handFunc != nil {
				handFunc(writer, req)
			}
		} else {
			defaultHandle(writer, req)
		}
		break
	}
}

func Post(pluginId string, uri string, req any, result any) error {
	var pluginInfo *PluginInfo
	pluginInfo, ok := pluginMap.Get(pluginId)
	if !ok {
		return fmt.Errorf("pluginInfo %s does not exist", pluginId)
	}

	if pluginInfo.Status != PluginStatusRunning {
		return fmt.Errorf("pluginInfo %s not started", pluginId)
	}

	var reqBuf *bytes.Buffer
	if req != nil {
		reqData, err := json.Marshal(req)
		if err != nil {
			return err
		}
		reqBuf = bytes.NewBuffer(reqData)
	} else {
		reqBuf = bytes.NewBufferString("")
	}

	client := &http.Client{}
	//生成要访问的url
	url := fmt.Sprintf("%s/%s", pluginInfo.Endpoint, uri)

	//提交请求
	request, err := http.NewRequest("POST", url, reqBuf)

	//增加header选项
	request.Header.Add("Token", plugin.Token)
	request.Header.Add("PluginId", MasterId)
	request.Header.Add("Accept", "application/json")
	request.Header.Add("Content-Type", "application/json")

	if err != nil {
		return err
	}
	//处理返回结果
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Error(err)
		}
	}(response.Body)
	if response.StatusCode != 200 {
		errResult := new(httputil2.Result[any])
		data, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		err = json.Unmarshal(data, result)
		if err != nil {
			return errors.New(string(data))
		}
		return fmt.Errorf("code:%d,msg:%s", errResult.Code, errResult.Message)
	}
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

func heartbeat() {
	go func() {
		for true {
			time.Sleep(5 * time.Minute)
			now := time.Now().UnixMilli()
			pluginMap.IterCb(func(k string, info *PluginInfo) {
				if info.Status == PluginStatusRunning {
					if info.AutoExit && info.Connections == 0 && (now-info.LastConnTime) >= AutoExitTimeLimit {
						//达到自动退出的条件(允许自动退出且没有进行中的连接且超过5分钟未进行连接)
						if !plugin.Debug {
							_ = StopPlugin(info.Id)
						}
					} else {
						go func() {
							//异步运行防止堵塞
							err := Post(info.Id, "ping", nil, nil)
							if err != nil {
								//如果响应失败则说明插件未运行,更新其状态
								_ = StopPlugin(info.Id)
							}
						}()

					}
				}
			})
		}
	}()
}

func pluginCoDeath() {
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		<-ch
		logger.Error("The main process is dead")
		//当监听到主进程关闭,尽量将子插件一同关闭
		pluginMap.IterCb(func(k string, info *PluginInfo) {
			if info.Status == PluginStatusRunning {
				go func() {
					//异步运行防止来不及干掉其他进程
					_ = StopPlugin(info.Id)
				}()
			}
		})
	}()
}
