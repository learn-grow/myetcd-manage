package v1

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"fmt"
	"myetcd-manage/program/config"
	"strings"
	"errors"
	"myetcd-manage/program/etcdv3"
	"encoding/json"
	"time"
	"os"
	"bufio"
	"myetcd-manage/program/logger"
	"strconv"
	"myetcd-manage/program/common"
)

// V1 v1 版接口
func V1(v1 *gin.RouterGroup){
	v1.GET("/members", getEtcdMembers) // 获取节点列表

	v1.GET("/server", getEtcdServerList) // 获取etcd服务列表

	// key的操作
	v1.POST("/key", postEtcdKey)       // 添加key
	v1.GET("/list", getEtcdKeyList)    // 获取目录下列表
	v1.GET("/key", getEtcdKeyValue)    // 获取一个key的具体值
	v1.PUT("/key", putEtcdKey)         // 修改key
	v1.DELETE("/key", delEtcdKey)      // 删除key

	//展示形式
	v1.GET("/key/format", getValueToFormat) // 格式化为json或toml


	//日志
	v1.GET("/logs", getLogsList) // 查询日志


	v1.GET("/users", getUserList)       // 获取用户列表
	v1.GET("/logtypes", getLogTypeList) // 获取日志类型列表


}

//--------------------日志-------------------



// 获取操作类型列表
func getLogTypeList(c *gin.Context) {
	c.JSON(http.StatusOK, []string{
		"获取列表",
		"获取key的值",
		"获取etcd集群信息",
		"删除key",
		"保存key",
		"获取etcd服务列表",
	})
}

// 获取用户列表
func getUserList(c *gin.Context) {
	us := make([]map[string]string, 0)
	cfg := config.GetCfg()
	if cfg != nil {
		for _, v := range cfg.Users {
			us = append(us, map[string]string{
				"name": v.Username,
				"role": v.Role,
			})
		}
	}

	c.JSON(http.StatusOK, us)
}



//日志信息
type LogLine struct {
	Date  string  `json:"date"`
	User  string  `json:"user"`
	Role  string  `json:"role"`
	Msg   string  `json:"msg"`
	Ts    float64 `json:"ts"`
	Level string  `json:"level"`
}

// 查看日志
func getLogsList(c *gin.Context) {
	page := c.Query("page")
	pageSize := c.Query("page_size")
	dateStr := c.Query("date")
	querUser := c.Query("user")
	queryLogType := c.Query("log_type")

	var err error
	defer func() {
		if err != nil {
			logger.Log.Errorw("查看日志错误", "err", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"msg": err.Error(),
			})
		}
	}()

	// 计算开始和结束行
	pageNum, _ := strconv.Atoi(page)
	if pageNum <= 0 {
		pageNum = 1
	}
	pageSizeNum, _ := strconv.Atoi(pageSize)
	if pageSizeNum <= 0 {
		pageSizeNum = 10
	}
	startLine := (pageNum - 1) * pageSizeNum
	endLine := pageNum * pageSizeNum

	fileName := fmt.Sprintf("%slogs/%s.log", common.GetRootDir(), dateStr)
	// fmt.Println(fileName)
	// 判断文件是否存在
	if exists, err := common.PathExists(fileName); exists == false || err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"msg": fmt.Sprintf("[%s]没有日志", dateStr),
		})
		return
	}
	// 读取指定行
	file, err := os.Open(fileName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"msg": "读取日志文件错误",
		})
		return
	}
	defer file.Close()
	fileScanner := bufio.NewScanner(file)
	lineCount := 1
	list := make([]*LogLine, 0) // 最终数组
	for fileScanner.Scan() {
		logTxt := fileScanner.Text()
		if logTxt == "" {
			continue
		}
		// 解析日志
		oneLog := new(LogLine)
		err = json.Unmarshal([]byte(logTxt), oneLog)
		if err != nil {
			logger.Log.Errorw("解析日志文件错误", "err", err)
			continue
		}
		// 只看info类型日志
		if oneLog.Level != "info" {
			continue
		}

		if lineCount > startLine && lineCount <= endLine {
			// 判断用户和日志类型参数
			if querUser != "" && oneLog.User != querUser {
				continue
			}
			if queryLogType != "" && oneLog.Msg != queryLogType {
				continue
			}

			oneLog.Date = time.Unix(int64(oneLog.Ts), 0).In(time.Local).Format("2006-01-02 15:04:05")
			list = append(list, oneLog)
		}

		lineCount++
	}
	err = nil

	c.JSON(http.StatusOK, gin.H{
		"list":  list,
		"total": lineCount - 1,
	})
}




// 记录访问日志
func saveLog(c *gin.Context, msg string) {
	user := c.MustGet(gin.AuthUserKey).(string) // 用户
	userRole := ""                              // 角色
	userRoleIn, exists := c.Get("userRole")
	if exists == true && userRoleIn != nil {
		userRole = userRoleIn.(string)
	}
	// 存储日志
	logger.Log.Infow(msg, "user", user, "role", userRole)
}



//--------------------json格式展示-------------------


// 获取key前缀，下的值为指定格式 josn toml
func getValueToFormat(c *gin.Context) {
	go saveLog(c, "格式化显示key")

	format := c.Query("format")
	key := c.Query("key")

	var err error
	defer func() {
		if err != nil {
			logger.Log.Errorw("保存key错误", "err", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"msg": err.Error(),
			})
		}
	}()

	etcdCli, exists := c.Get("EtcdServer")
	if exists == false {
		err = errors.New("Etcd client is empty")
		return
	}
	cli := etcdCli.(*etcdv3.Etcd3Client)

	list, err := cli.GetRecursiveValue(key)
	if err != nil {
		return
	}

	// js, _ := json.Marshal(list)
	// log.Println(string(js))

	switch format {
	case "json":
		resp, err := etcdv3.NodeJsonFormat(key, list)
		if err != nil {
			return
		}
		respJs, _ := json.MarshalIndent(resp, "", "    ")
		c.JSON(http.StatusOK, string(respJs))
		return
	case "toml":

	default:
		err = errors.New("不支持的格式")
	}
}




//--------------------删除key-------------------

// 删除key
func delEtcdKey(c *gin.Context) {
	go saveLog(c, "删除key")

	key := c.Query("key")

	var err error
	defer func() {
		if err != nil {
			logger.Log.Errorw("删除key错误", "err", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"msg": err.Error(),
			})
		}
	}()

	etcdCli, exists := c.Get("EtcdServer")
	if exists == false {
		c.JSON(http.StatusBadRequest, gin.H{
			"msg": "Etcd client is empty",
		})
		return
	}
	cli := etcdCli.(*etcdv3.Etcd3Client)

	err = cli.Delete(key)
	if err != nil {
		return
	}

	c.JSON(http.StatusOK, "ok")
}



//--------------------修改key-------------------
// 修改key
func putEtcdKey(c *gin.Context) {
	saveEtcdKey(c, true)
}




// 获取key的值
func getEtcdKeyValue(c *gin.Context) {
	go saveLog(c, "获取key的值")

	key := c.Query("key")
	var err error
	defer func() {
		if err != nil {
			logger.Log.Errorw("获取key值的值错误", "err", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"msg": err.Error(),
			})
		}
	}()

	etcdCli, exists := c.Get("EtcdServer")
	if exists == false {
		c.JSON(http.StatusBadRequest, gin.H{
			"msg": "Etcd client is empty",
		})
		return
	}
	cli := etcdCli.(*etcdv3.Etcd3Client)

	val, err := cli.Value(key)
	if err != nil {
		return
	}

	c.JSON(http.StatusOK, val)
}

//-------------------------获取列表--------------------------

// 获取etcd key列表
func getEtcdKeyList(c *gin.Context) {
	go saveLog(c, "获取列表")

	key := c.Query("key")

	var err error
	defer func() {
		if err != nil {
			logger.Log.Errorw("获取key列表错误", "err", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"msg": err.Error(),
			})
		}
	}()

	// log.Println(key)
	etcdCli, exists := c.Get("EtcdServer")
	fmt.Println("etcdCli,",etcdCli)
	if exists == false {
		c.JSON(http.StatusBadRequest, gin.H{
			"msg": "Etcd client is empty",
		})
		return
	}
	cli := etcdCli.(*etcdv3.Etcd3Client)

	resp, err := cli.List(key)
	if err != nil {
		return
	}

	list := make([]*etcdv3.Node, 0)
	for _, v := range resp {
		if v.FullDir != "/" {
			list = append(list, v)
		}
	}

	c.JSON(http.StatusOK, list)
}

//---------------------添加key-------------------

// 添加key
func postEtcdKey(c *gin.Context) {
	saveEtcdKey(c, false)
}

// PostReq 添加和修改时的body
type PostReq struct {
	*etcdv3.Node
	EtcdName string `json:"etcd_name"`
}

// 保存key
func saveEtcdKey(c *gin.Context, isPut bool) {
	go saveLog(c, "保存key")

	var err error
	defer func() {
		if err != nil {
			logger.Log.Errorw("保存key错误", "err", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"msg": err.Error(),
			})
		}
	}()

	req := new(PostReq)
	err = c.Bind(req)
	if err != nil {
		return
	}
	if req.FullDir == "" {
		err = errors.New("参数错误")
		return
	}

	etcdCli, exists := c.Get("EtcdServer")
	if exists == false {
		err = errors.New("Etcd client is empty")
		return
	}
	cli := etcdCli.(*etcdv3.Etcd3Client)

	// 判断根目录是否存在
	rootDir := ""
	dirs := strings.Split(req.FullDir, "/")
	if len(dirs) > 1 {
		// 兼容/开头的key
		if req.FullDir[:1] == "/" {
			_, err = cli.Value("/")
			if err != nil {
				err = cli.Put("/", etcdv3.DEFAULT_DIR_VALUE, true)
				if err != nil {
					return
				}
			}
		}
		rootDir = strings.Join(dirs[:len(dirs)-1], "/")
	}
	if rootDir != "" {
		// 用/分割
		rootDirs := strings.Split(rootDir, "/")
		if len(rootDirs) > 1 {
			rootDir1 := ""
			for _, vDir := range rootDirs {
				if vDir == "" {
					vDir = "/"
				}
				if rootDir1 != "" && rootDir1 != "/" {
					rootDir1 += "/"
				}
				rootDir1 += vDir
				_, err = cli.Value(rootDir1)
				if err != nil {
					err = cli.Put(rootDir1, etcdv3.DEFAULT_DIR_VALUE, true)
					if err != nil {
						return
					}
				}
			}
		} else {
			_, err = cli.Value(rootDir)
			if err != nil {
				err = cli.Put(rootDir, etcdv3.DEFAULT_DIR_VALUE, true)
				if err != nil {
					return
				}
			}
		}
	}

	// 保存key
	if req.IsDir == true {
		if isPut == true {
			err = errors.New("目录不能修改")
		} else {
			err = cli.Put(req.FullDir, etcdv3.DEFAULT_DIR_VALUE, !isPut)
		}
	} else {
		err = cli.Put(req.FullDir, req.Value, !isPut)
	}

	if err != nil {
		return
	}

	c.JSON(http.StatusOK, "ok")
}




//-----------------------展示配置信息中的server信息---------------------------


// 获取etcd服务列表
func getEtcdServerList(c *gin.Context) {
	go saveLog(c, "获取etcd服务列表")

	cfg := config.GetCfg()
	if cfg == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"msg": "配置为nil",
		})
		return
	}
	list := cfg.Server
	if list == nil {
		list = make([]*config.EtcdServer, 0)
		c.JSON(http.StatusOK, list)
		return
	}
	// 当前用户角色
	userRole := ""
	userRoleIn, exists := c.Get("userRole")
	if exists == true {
		userRole = userRoleIn.(string)
	}

	// log.Println(userRole)

	// 只返回有权限服务列表
	list1 := make([]*config.EtcdServer, 0)
	for _, s := range list {
		if s.Roles == nil || len(s.Roles) == 0 {
			list1 = append(list1, s)
		} else {
			for _, r := range s.Roles {
				if r == userRole {
					list1 = append(list1, s)
					break
				}
			}
		}
	}

	c.JSON(http.StatusOK, list1)
}



// 获取服务节点
func getEtcdMembers(c *gin.Context) {
	go saveLog(c, "获取etcd集群信息")

	var err error
	defer func() {
		if err != nil {
			logger.Log.Errorw("获取服务节点错误", "err", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"msg": err.Error(),
			})
		}
	}()

	etcdCli, exists := c.Get("EtcdServer")
	if exists == false {
		fmt.Println("-->Etcd client is empty")
		c.JSON(http.StatusBadRequest, gin.H{
			"msg": "Etcd client is empty",
		})
		return
	}
	cli := etcdCli.(*etcdv3.Etcd3Client)

	members, err := cli.Members()
	fmt.Println("---->>>,len:",len(members))
	if err != nil {
		return
	}

	c.JSON(http.StatusOK, members)
}