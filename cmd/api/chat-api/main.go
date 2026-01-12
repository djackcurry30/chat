// Copyright © 2023 OpenIM open source community. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// 主包声明 - 聊天API服务的入口点
package main

import (
	// 引入命令行工具包，包含ChatApiCmd的定义
	"github.com/openimsdk/chat/pkg/common/cmd"
	// 引入程序退出处理工具
	"github.com/openimsdk/tools/system/program"
)

// 主函数 - 程序执行的起始点
// 功能：创建并执行ChatApi命令，启动聊天API服务
func main() {
	// 1. 创建ChatApi命令实例
	// 2. 执行命令，启动API服务
	// 3. 如果执行过程中发生错误，调用ExitWithError退出程序并返回错误码
	if err := cmd.NewChatApiCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}
