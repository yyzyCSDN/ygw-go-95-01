基于 Go 实现的港口岸电系统项目，一款港口供电控制服务，完成船舶岸电接入、并网切换、容量分配与计量计费联动管理。

系统面向港口靠泊船舶的岸电作业：调度登记泊位与船籍用电参数，值班员发起岸电接入，系统执行泊位占用、容量分配、绝缘与相位核对、并网合闸、计量采样，离泊时解列并结算。

## 模块

- berth：泊位台账与占用状态管理
- capacity：岸电容量分配、回收与定时核对
- connect：接入流程编排与校验确认
- grid：并网同期判定与断路器控制
- meter：计量采样、峰谷电价与结算
- alarm：告警分级与联动处置
- record：运行记录持久化与指纹去重
- event：进程内事件总线

## 构建与运行

前置：Go 1.23，GOTOOLCHAIN=local，依赖已 vendor 离线。

```bash
go build -mod=vendor ./...
go vet -mod=vendor ./...
go test -mod=vendor ./...
```

启动服务：

```bash
go run -mod=vendor ./cmd/server -config config.example.json
```

默认监听 127.0.0.1:8095，浏览器打开 http://127.0.0.1:8095/ 进入监控台。

## 接口

- GET /api/berths 泊位列表
- POST /api/berths/occupy 登记靠泊
- POST /api/berths/depart 离泊回收
- GET /api/capacity/plan 容量分配计划
- POST /api/connect/apply 发起岸电接入
- POST /api/connect/confirm 确认校验完成
- POST /api/connect/wait 等待校验结果
- POST /api/connect/cancel 撤销接入
- GET /api/grid/state 并网状态
- POST /api/grid/separate 解列
- POST /api/grid/switch 并网切换序列
- GET /api/meter/samples 计量运行
- POST /api/meter/settle 结算
- GET /api/alarms 告警列表
- POST /api/control/override 就地操作生效
- POST /api/control/dispatch 自动并网下发
- GET /api/snapshot 全量快照
- WS /ws/console 实时状态推送
