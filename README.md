# Go Finite State Machine (FSM) Library

一个生产就绪、类型安全的Go有限状态机库，支持层次状态、并行状态、守卫条件、动作等高级特性。

## 特性概览

- ✅ **类型安全** - 基于Go泛型的编译时类型检查
- ✅ **层次状态** - 支持嵌套的复合状态
- ✅ **并行状态** - 多个状态同时激活
- ✅ **守卫条件** - 条件化的状态转移
- ✅ **入口/出口动作** - 状态进入和退出时的回调
- ✅ **异步处理** - 事件队列和并发安全
- ✅ **历史状态** - 支持浅历史和深历史
- ✅ **状态持久化** - 状态快照和恢复
- ✅ **事件监听** - 完整的状态变更通知
- ✅ **线程安全** - 所有操作都是并发安全的

## 快速开始

### 安装

```bash
go get github.com/dongrv/fsm-go
```

### 基本用法

```go
package main

import (
    "fmt"
    "github.com/dongrv/fsm-go"
)

func main() {
    // 创建一个简单的门状态机
    machine := fsm.NewBuilder[string, string]().
        AddState("closed").
        AddState("opened").
        AddTransition("closed", "opened", "open").
        AddTransition("opened", "closed", "close").
        Build()

    // 启动状态机
    if err := machine.Start("closed"); err != nil {
        panic(err)
    }

    // 发送事件
    fmt.Println("初始状态:", machine.Current()) // closed
    
    machine.Send("open")
    fmt.Println("开门后:", machine.Current()) // opened
    
    machine.Send("close")
    fmt.Println("关门后:", machine.Current()) // closed

    // 停止状态机
    machine.Stop()
}
```

## 核心概念

### 1. 状态（States）

状态可以是简单的，也可以是复合的：

```go
// 简单状态
machine.AddState("idle")

// 带入口动作的状态
machine.AddState("active", fsm.WithEntryAction(func(ctx fsm.Context[string, string]) {
    fmt.Println("进入激活状态")
}))

// 最终状态
machine.AddState("completed", fsm.AsFinal[string, string]())

// 带超时的状态
machine.AddState("waiting", fsm.WithTimeout(5*time.Second, "timeout"))
```

### 2. 转移（Transitions）

转移定义了状态之间的变化：

```go
// 基本转移
machine.AddTransition("idle", "running", "start")

// 带守卫条件的转移
machine.AddTransition("locked", "unlocked", "unlock",
    fsm.WithGuard(func(ctx fsm.Context[string, string]) bool {
        // 检查密码是否正确
        key, ok := ctx.Payload().(string)
        return ok && key == "1234"
    }))

// 带动作的转移
machine.AddTransition("running", "stopped", "stop",
    fsm.WithAction(func(ctx fsm.Context[string, string]) {
        fmt.Printf("从 %s 转移到 %s\n", ctx.From(), ctx.To())
    }))

// 内部转移（不改变状态）
machine.AddInternalTransition("running", "pause",
    fsm.WithAction(func(ctx fsm.Context[string, string]) {
        fmt.Println("暂停运行")
    }))
```

### 3. 层次状态

支持嵌套的复合状态：

```go
machine := fsm.NewExtendedBuilder[string, string]().
    AddState("off").
    AddCompositeState("on", func(b *fsm.CompositeBuilder[string, string]) {
        b.AddState("idle").
            WithEntry(func(ctx fsm.Context[string, string]) {
                fmt.Println("设备空闲")
            }).
            End()
        
        b.AddCompositeState("washing", func(b2 *fsm.CompositeBuilder[string, string]) {
            b2.AddState("filling").
                WithEntry(func(ctx fsm.Context[string, string]) {
                    fmt.Println("注水中...")
                }).
                End()
            
            b2.AddState("washing").
                WithEntry(func(ctx fsm.Context[string, string]) {
                    fmt.Println("洗涤中...")
                }).
                End()
            
            b2.WithInitial("filling")
        }).
        WithEntry(func(ctx fsm.Context[string, string]) {
            fmt.Println("开始洗涤循环")
        }).
        End()
        
        b.WithInitial("idle")
    }).
    Build()
```

### 4. 事件监听

监听状态变化：

```go
listener := &fsm.SimpleListener[string, string]{
    OnTransitionFunc: func(ctx fsm.Context[string, string]) {
        fmt.Printf("转移: %s -> %s (事件: %s)\n",
            ctx.From(), ctx.To(), ctx.Event())
    },
    OnEntryFunc: func(state string, event string) {
        fmt.Printf("进入状态: %s\n", state)
    },
    OnExitFunc: func(state string, event string) {
        fmt.Printf("退出状态: %s\n", state)
    },
    OnEventFunc: func(event string, payload any) {
        fmt.Printf("收到事件: %s\n", event)
    },
    OnErrorFunc: func(err error) {
        fmt.Printf("错误: %v\n", err)
    },
}

machine.AddListener(listener)
```

## 实用示例

### 示例1：交通灯控制器

```go
func TrafficLight() *fsm.Machine[string, string] {
    return fsm.NewBuilder[string, string]().
        AddState("red").
        AddState("green").
        AddState("yellow").
        AddTransition("red", "green", "next").
        AddTransition("green", "yellow", "next").
        AddTransition("yellow", "red", "next").
        Build()
}
```

### 示例2：重试机制

```go
func RetryPattern() *fsm.Machine[string, string] {
    return fsm.NewExtendedBuilder[string, string]().
        AddState("idle").
        AddState("working").
        AddState("retrying").
        AddState("failed", fsm.AsFinal[string, string]()).
        AddTransition("idle", "working", "start").
        AddTransition("working", "idle", "success").
        AddTransition("working", "retrying", "fail").
        AddTransition("retrying", "working", "retry").
        AddTransition("retrying", "failed", "fail").
        Build()
}
```

### 示例3：断路器模式

```go
func CircuitBreaker() *fsm.Machine[string, string] {
    return fsm.NewExtendedBuilder[string, string]().
        AddState("closed").    // 正常操作
        AddState("open").      // 断路器打开
        AddState("halfOpen").  // 半开状态（测试）
        AddTransition("closed", "open", "failure").
        AddTransition("open", "halfOpen", "timeout").
        AddTransition("halfOpen", "closed", "success").
        AddTransition("halfOpen", "open", "failure").
        Build()
}
```

## 高级特性

### 异步事件处理

```go
machine := fsm.NewBuilder[string, string]().
    WithAsync().          // 启用异步模式
    WithQueueSize(1000).  // 设置事件队列大小
    AddState("ready").
    AddState("processing").
    AddTransition("ready", "processing", "process").
    Build()
```

### 状态持久化

```go
// 保存状态快照
snapshot, err := machine.Save()
if err != nil {
    // 处理错误
}

// 恢复状态
err = machine.Load(snapshot)
if err != nil {
    // 处理错误
}
```

### 状态数据存储

```go
// 存储状态相关数据
machine.SetStateData("user", User{ID: 123, Name: "Alice"})

// 获取状态数据
data, exists := machine.GetStateData("user")
if exists {
    user := data.(User)
    fmt.Println("用户:", user.Name)
}
```

## 错误处理

所有可能失败的操作都返回错误：

```go
// 启动状态机
if err := machine.Start("initial"); err != nil {
    // 处理错误：状态不存在、已初始化等
}

// 发送事件
if err := machine.Send("someEvent"); err != nil {
    // 处理错误：无对应转移、队列已满等
}

// 检查事件是否可处理
if machine.Can("someEvent") {
    // 安全发送事件
    machine.Send("someEvent")
}
```

## 性能优化建议

1. **同步 vs 异步**
   - 同步模式：低延迟，适合简单状态机
   - 异步模式：高吞吐，适合复杂或I/O密集型场景

2. **事件队列大小**
   - 根据业务负载调整队列大小
   - 避免队列过小导致事件丢失
   - 避免队列过大导致内存占用

3. **监听器优化**
   - 监听器操作应尽量快速
   - 避免在监听器中执行阻塞操作
   - 考虑使用缓冲通道处理监听事件

## 常见问题

### Q: 如何定义自定义状态和事件类型？
A: 任何实现了 `comparable` 接口的类型都可以作为状态或事件：

```go
type StateType int
type EventType string

machine := fsm.NewBuilder[StateType, EventType]().
    AddState(StateType(0)).
    AddState(StateType(1)).
    AddTransition(StateType(0), StateType(1), EventType("next")).
    Build()
```

### Q: 如何处理并发访问？
A: 所有公共方法都是线程安全的，可以安全地在多个goroutine中调用。

### Q: 状态机可以动态修改吗？
A: 状态机一旦构建完成就是不可变的。如果需要动态行为，可以：
1. 重新构建状态机
2. 使用守卫条件实现动态逻辑
3. 使用动作修改外部状态

### Q: 如何调试状态机？
A: 使用监听器记录所有状态变化：

```go
listener := &fsm.SimpleListener[string, string]{
    OnTransitionFunc: func(ctx fsm.Context[string, string]) {
        log.Printf("[DEBUG] %s -> %s (%s)", 
            ctx.From(), ctx.To(), ctx.Event())
    },
}
```

## 示例代码

查看 `examples/` 目录获取完整示例：
- `simple/` - 基础状态机示例
- `hierarchical/` - 层次状态示例
- `guards/` - 守卫条件示例

## 贡献指南

欢迎提交Issue和Pull Request！请确保：
1. 代码符合Go代码规范
2. 添加相应的测试用例
3. 更新相关文档

## 许可证

MIT License - 详见 LICENSE 文件
