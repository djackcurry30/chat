// chat包 - 聊天API服务的核心实现
package chat

import (
	"context"   // 上下文管理
	"errors"    // 错误处理
	"fmt"       // 格式化输出
	"net/http"  // HTTP服务器
	"os"        // 操作系统接口
	"os/signal" // 信号处理
	"syscall"   // 系统调用
	"time"      // 时间处理

	"github.com/gin-gonic/gin"                                 // Gin Web框架
	chatmw "github.com/openimsdk/chat/internal/api/mw"         // 聊天API中间件
	"github.com/openimsdk/chat/internal/api/util"              // API工具函数
	"github.com/openimsdk/chat/pkg/common/config"              // 配置管理
	"github.com/openimsdk/chat/pkg/common/imapi"               // OpenIM API客户端
	"github.com/openimsdk/chat/pkg/common/kdisc"               // 服务发现客户端
	disetcd "github.com/openimsdk/chat/pkg/common/kdisc/etcd"  // etcd配置管理
	adminclient "github.com/openimsdk/chat/pkg/protocol/admin" // 管理员RPC客户端
	chatclient "github.com/openimsdk/chat/pkg/protocol/chat"   // 聊天RPC客户端
	"github.com/openimsdk/tools/discovery/etcd"                // etcd服务发现实现
	"github.com/openimsdk/tools/errs"                          // 错误包装工具
	"github.com/openimsdk/tools/mw"                            // 通用中间件
	"github.com/openimsdk/tools/system/program"                // 程序管理工具
	"github.com/openimsdk/tools/utils/datautil"                // 数据工具
	"github.com/openimsdk/tools/utils/runtimeenv"              // 运行时环境工具
	"google.golang.org/grpc"                                   // gRPC客户端库
	"google.golang.org/grpc/credentials/insecure"              // 不安全的gRPC凭证
)

// Config 聊天API服务配置结构体
// 包含API配置、服务发现配置和共享配置
// RuntimeEnv: 运行时环境标识

type Config struct {
	ApiConfig config.API       // API服务配置
	Discovery config.Discovery // 服务发现配置
	Share     config.Share     // 共享配置

	RuntimeEnv string // 运行时环境标识
}

// Start 启动聊天API服务
// 参数：
//   - ctx: 上下文，用于传递全局配置和控制服务生命周期
//   - index: 服务索引，用于多实例部署时选择端口
//   - cfg: 配置结构体指针，包含服务所需的所有配置
//
// 返回值：
//   - error: 启动过程中的错误，启动成功则返回nil
//
// 功能：
//  1. 打印运行时环境信息
//  2. 验证配置完整性
//  3. 获取API端口
//  4. 创建服务发现客户端
//  5. 建立gRPC连接
//  6. 初始化各种客户端
//  7. 设置Gin引擎和路由
//  8. 启动HTTP服务器
//  9. 监听配置变化
//  10. 设置优雅关闭
//  11. 处理系统信号
func Start(ctx context.Context, index int, cfg *Config) error {
	// 打印运行时环境信息，并将环境标识保存到配置中
	cfg.RuntimeEnv = runtimeenv.PrintRuntimeEnvironment()

	// 验证聊天管理员配置是否存在
	if len(cfg.Share.ChatAdmin) == 0 {
		return errs.New("共享配置中未配置聊天管理员")
	}

	// 根据服务索引获取API端口
	apiPort, err := datautil.GetElemByIndex(cfg.ApiConfig.Api.Ports, index)
	if err != nil {
		return err
	}

	// 创建服务发现注册客户端
	client, err := kdisc.NewDiscoveryRegister(&cfg.Discovery, cfg.RuntimeEnv, nil)
	if err != nil {
		return err
	}

	// 建立与聊天RPC服务的gRPC连接
	chatConn, err := client.GetConn(ctx, cfg.Discovery.RpcService.Chat,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		mw.GrpcClient())
	if err != nil {
		return err
	}

	// 建立与管理员RPC服务的gRPC连接
	adminConn, err := client.GetConn(ctx, cfg.Discovery.RpcService.Admin,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		mw.GrpcClient())
	if err != nil {
		return err
	}

	// 创建聊天RPC客户端
	chatClient := chatclient.NewChatClient(chatConn)
	// 创建管理员RPC客户端
	adminClient := adminclient.NewAdminClient(adminConn)
	// 创建OpenIM API客户端
	im := imapi.New(cfg.Share.OpenIM.ApiURL, cfg.Share.OpenIM.Secret, cfg.Share.OpenIM.AdminUserID)

	// 初始化API基础配置
	base := util.Api{
		ImUserID:        cfg.Share.OpenIM.AdminUserID, // OpenIM管理员用户ID
		ProxyHeader:     cfg.Share.ProxyHeader,        // 代理头配置
		ChatAdminUserID: cfg.Share.ChatAdmin[0],       // 聊天管理员用户ID
	}

	// 创建API实例
	adminApi := New(chatClient, adminClient, im, &base)
	// 创建中间件实例
	mwApi := chatmw.New(adminClient)

	// 设置Gin模式为生产模式
	gin.SetMode(gin.ReleaseMode)
	// 创建Gin引擎
	engine := gin.New()
	// 添加全局中间件：恢复中间件、CORS处理、操作ID解析
	engine.Use(gin.Recovery(), mw.CorsHandler(), mw.GinParseOperationID())
	// 设置聊天API路由
	SetChatRoute(engine, adminApi, mwApi)

	// 定义网络完成通道和网络错误变量
	var (
		netDone = make(chan struct{}, 1) // 用于通知网络服务异常退出
		netErr  error                    // 存储网络服务错误
	)

	// 创建HTTP服务器
	server := http.Server{Addr: fmt.Sprintf(":%d", apiPort), Handler: engine}

	// 启动HTTP服务器（异步）
	go func() {
		err = server.ListenAndServe()
		// 检查服务器是否正常关闭
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			netErr = errs.WrapMsg(err, fmt.Sprintf("API服务启动失败: %s", server.Addr))
			netDone <- struct{}{} // 通知主协程服务器异常退出
		}
	}()

	// 如果启用了etcd服务发现，创建配置管理器并监听配置变化
	if cfg.Discovery.Enable == kdisc.ETCDCONST {
		cm := disetcd.NewConfigManager(
			client.(*etcd.SvcDiscoveryRegistryImpl).GetClient(),
			[]string{
				config.ChatAPIChatCfgFileName,  // 聊天API配置文件
				config.DiscoveryConfigFileName, // 服务发现配置文件
				config.ShareFileName,           // 共享配置文件
				config.LogConfigFileName,       // 日志配置文件
			},
		)
		cm.Watch(ctx) // 监听配置变化
	}

	// 定义优雅关闭函数
	shutdown := func() error {
		// 创建15秒超时上下文，用于控制关闭过程
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		// 关闭HTTP服务器
		err := server.Shutdown(ctx)
		if err != nil {
			return errs.WrapMsg(err, "服务器关闭失败")
		}
		return nil
	}

	// 注册关闭函数到etcd配置管理器
	disetcd.RegisterShutDown(shutdown)

	// 监听系统SIGTERM信号，用于优雅关闭
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)

	// 等待信号或网络错误
	select {
	case <-sigs: // 收到SIGTERM信号
		program.SIGTERMExit()              // 打印退出信息
		if err := shutdown(); err != nil { // 执行优雅关闭
			return err
		}
	case <-netDone: // 网络服务异常退出
		close(netDone) // 关闭通道
		return netErr  // 返回网络服务错误
	}

	return nil
}

// SetChatRoute 设置聊天API路由
// 参数：
//   - router: Gin路由接口
//   - chat: API处理器实例
//   - mw: 中间件实例
//
// 功能：
//
//	配置所有聊天API的路由规则和处理函数
func SetChatRoute(router gin.IRouter, chat *Api, mw *chatmw.MW) {
	// 账号相关路由组
	account := router.Group("/account")
	account.POST("/code/send", chat.SendVerifyCode)                      // 发送验证码
	account.POST("/code/verify", chat.VerifyCode)                        // 验证验证码
	account.POST("/register", mw.CheckAdminOrNil, chat.RegisterUser)     // 注册用户（需要管理员权限或开放注册）
	account.POST("/login", chat.Login)                                   // 用户登录
	account.POST("/password/reset", chat.ResetPassword)                  // 重置密码
	account.POST("/password/change", mw.CheckToken, chat.ChangePassword) // 修改密码（需要token验证）

	// 用户相关路由组（需要token验证）
	user := router.Group("/user", mw.CheckToken)
	user.POST("/update", chat.UpdateUserInfo)                 // 更新用户信息
	user.POST("/find/public", chat.FindUserPublicInfo)        // 获取用户公开信息
	user.POST("/find/full", chat.FindUserFullInfo)            // 获取用户完整信息
	user.POST("/search/full", chat.SearchUserFullInfo)        // 搜索用户公开信息
	user.POST("/search/public", chat.SearchUserPublicInfo)    // 搜索用户完整信息
	user.POST("/rtc/get_token", chat.GetTokenForVideoMeeting) // 获取视频会议token

	// 好友相关路由（需要token验证）
	router.POST("/friend/search", mw.CheckToken, chat.SearchFriend) // 搜索好友

	// 小程序相关路由（需要token验证）
	router.Group("/applet").POST("/find", mw.CheckToken, chat.FindApplet) // 获取小程序列表

	// 客户端配置路由
	router.Group("/client_config").POST("/get", chat.GetClientConfig) // 获取客户端初始化配置

	// 应用版本相关路由
	applicationGroup := router.Group("application")
	applicationGroup.POST("/latest_version", chat.LatestApplicationVersion) // 获取最新应用版本
	applicationGroup.POST("/page_versions", chat.PageApplicationVersion)    // 分页获取应用版本

	// OpenIM回调路由
	router.Group("/callback").POST("/open_im", chat.OpenIMCallback) // 处理OpenIM回调
}
