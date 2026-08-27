# 水滴记录（WaterDrop Drink Tracker）

一个轻量的 Windows 桌面喝水记录工具，数据只保存在本机，不需要登录。

## 下载

- 直接下载：[`WaterDrop.exe`](./WaterDrop.exe)
- 压缩包：[`WaterDrop-Windows-x64.zip`](./WaterDrop-Windows-x64.zip)

支持 Windows 10 / Windows 11（64 位）。程序为单文件便携版，下载后双击即可运行，无需安装。

> 程序暂未购买代码签名证书。Windows 首次运行若出现 SmartScreen 提示，请选择“更多信息”→“仍要运行”。

## 功能

- 桌面置顶悬浮小窗
- 本地记录，无账号、无网络请求
- 每日喝水目标设定
- 150 / 250 / 350 / 500 ml 快速记录，整个按钮均可点击
- 今日记录查看与删除
- 历史日历按天显示总喝水量
- 数据保存在 `%APPDATA%\WaterDrop\water.json`

## 从源码构建

需要 Go 1.26 或更高版本：

```powershell
go build -trimpath -ldflags="-H windowsgui -s -w" -o WaterDrop.exe .
```

项目使用纯 Win32 API 和 Go 标准库，不依赖额外运行时。
