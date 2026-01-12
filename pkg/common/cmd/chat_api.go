// cmd包 - 定义聊天服务的命令行工具
package cmd

import (
	"context" // 上下文管理，用于传递请求范围的数据

	// 引入聊天API的具体实现
	"github.com/openimsdk/chat/internal/api/chat"
	// 引入配置管理包
	"github.com/openimsdk/chat/pkg/common/config"
	// 引入程序工具包，用于获取进程名称
	"github.com/openimsdk/tools/system/program"
	// 引入cobra命令行框架
	"github.com/spf13/cobra"
)

// ChatApiCmd 聊天API命令结构体
// 功能：封装聊天API服务的启动命令和相关配置
// 继承自RootCmd，扩展了聊天API特有的配置和上下文

type ChatApiCmd struct {
	*RootCmd                  // 嵌入RootCmd，继承其基本功能
	ctx       context.Context // 上下文，用于传递全局配置和元数据
	configMap map[string]any  // 配置映射，将配置文件与结构体字段关联
	apiConfig chat.Config     // 聊天API的具体配置结构体
}

// NewChatApiCmd 创建聊天API命令实例
// 返回值：ChatApiCmd指针，用于启动聊天API服务
// 功能：初始化命令配置，设置配置文件映射关系，注册执行函数
func NewChatApiCmd() *ChatApiCmd {
	// 创建ChatApiCmd实例
	var ret ChatApiCmd

	// 初始化配置映射，将配置文件与apiConfig结构体字段关联
	// 配置文件会自动加载到对应的结构体字段中
	ret.configMap = map[string]any{
		// 共享配置文件映射到apiConfig.Share
		config.ShareFileName: &ret.apiConfig.Share,
		// 聊天API配置文件映射到apiConfig.ApiConfig
		config.ChatAPIChatCfgFileName: &ret.apiConfig.ApiConfig,
		// 服务发现配置文件映射到apiConfig.Discovery
		config.DiscoveryConfigFileName: &ret.apiConfig.Discovery,
	}

	// 创建RootCmd实例，传入进程名称和配置映射选项
	ret.RootCmd = NewRootCmd(program.GetProcessName(), WithConfigMap(ret.configMap))

	// 创建上下文，包含版本信息
	ret.ctx = context.WithValue(context.Background(), "version", config.Version)

	// 注册命令执行函数
	ret.Command.RunE = func(cmd *cobra.Command, args []string) error {
		return ret.runE()
	}

	// 返回初始化完成的ChatApiCmd实例
	return &ret
}

// Exec 执行聊天API命令
// 返回值：error，如果执行失败则返回错误信息
// 功能：调用父类的Execute方法，启动命令行执行流程
func (a *ChatApiCmd) Exec() error {
	return a.Execute()
}

// runE 实际执行聊天API服务启动的函数
// 返回值：error，如果启动失败则返回错误信息
// 功能：调用chat.Start函数，启动聊天API服务
// 参数：
//   - a.ctx: 上下文，包含版本信息
//   - a.Index(): 服务索引，用于多实例部署
//   - &a.apiConfig: 聊天API配置，包含共享配置、API配置和服务发现配置
func (a *ChatApiCmd) runE() error {
	return chat.Start(a.ctx, a.Index(), &a.apiConfig)
}
