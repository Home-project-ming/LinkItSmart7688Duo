<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# LinkIt Smart 7688 Duo

[English](README.md)

這是 [Seeed / MediaTek LinkIt Smart 7688 Duo](https://www.seeedstudio.com/LinkIt-Smart-7688-Duo.html)
的硬體開發筆記,以及一個給它用的 MCP(Model Context Protocol)server。這塊板子上有**兩顆獨立的處理器**:MediaTek **MT7688AN**(MIPS24KEc,跑 OpenWrt Linux)跟 Atmel **ATmega32U4**(裸機 AVR),兩者透過 UART 橋接在一起。

這個 repo 一半是「怎麼在一塊 2016 年、資源很緊的板子上真的把事情做起來」的實戰記錄,一半是一個能實際運作的工具——讓 AI coding agent(Claude Code)透過 MCP 工具直接操作板子的 GPIO 跟 I2C,不用再手動開 SSH session。

## 這個 repo 裡有什麼

| 路徑 | 內容 |
|---|---|
| [`dev-notes.html`](dev-notes.html) | 敘事式筆記:把根檔案系統從 9.8 MB 的內建 flash 搬到 116 GiB 的 SD 卡、在 2016 年的韌體上裝現代 Python、以及一個至今沒解決的 SD 卡上動態連結執行檔 SIGILL/SIGSEGV 問題 |
| [`provisioning/`](provisioning/) | 每一個真的在板子上跑過的 shell 腳本,依照實際執行順序保留,讓整個過程在一塊全新的板子上可以重現,不用重新摸索一次。細節見 [`provisioning/README.md`](provisioning/README.md) |
| [`mt7688-mcp-server/`](mt7688-mcp-server/) | MCP server 本體——用 Go 寫、交叉編譯給板子用,把 `gpio_read`/`gpio_write`/`gpio_set_direction`/`i2c_read`/`i2c_write` 開放成 MCP client 能呼叫的工具 |
| [`docs/superpowers/specs/`](docs/superpowers/specs/) | 設計文件——記錄架構決策背後的理由,包括實作到一半才發現需要修正的部分 |
| [`docs/superpowers/plans/`](docs/superpowers/plans/) | 逐任務的實作計畫 |
| [`CLAUDE.md`](CLAUDE.md) | 給在這個 repo 裡工作的 AI coding agent 看的指引——板子架構、工具鏈、真正可用的 build/test/deploy 指令 |

## 這塊板子

| | MT7688 那側 | ATmega32U4 那側 |
|---|---|---|
| 晶片 | MediaTek MT7688AN,MIPS24KEc @ 580MHz | Microchip ATmega32U4,AVR @ 16MHz |
| 跑的系統 | OpenWrt 15.05.1(Chaos Calmer),kernel 3.18.23 | 裸機 / Arduino 韌體 |
| 負責的事 | 網路、WiFi、USB host、SD 卡、服務 | 即時 GPIO、ADC、PWM |

兩顆晶片之間靠 UART 接在一起——「Duo」這個名字的重點就在這裡。ATmega 那側感測或控制的任何東西,都得經過這條橋接線才能連到網路,反過來也一樣。

## 目前做到的事

- **Extroot 完成**:根檔案系統的可寫層從 9.8 MB 的內建 flash 搬到 116 GiB 的 SD 卡(空間大約多了一萬一千倍),重開機後仍然生效。
- **在 2016 年的韌體上跑現代 Python**:靠 Entware 的 `mipselsf-k3.4` target,自帶 glibc,跟系統的 uClibc 完全隔離——但有一個已經記錄下來、還沒解決的穩定性問題(見 `dev-notes.html`)。
- **抓到並修好一個真實的網路橋接迴圈**:WiFi 上行介面不小心跟有線網路埠橋接在同一個 L2 網段、又沒開 STP——這是直接在板子上現場診斷出來的,不是用猜的。
- **一個真的能用的 MCP server**,已經部署到真實硬體上驗證過:GPIO 跟 I2C 工具,寫死的安全黑名單(console UART、SD/extroot 控制線、ATmega 橋接 UART、ATmega 的 ISP reset 線)在碰硬體之前一定先檢查,而且抓到兩個只有在真實測試時才會浮現的 bug(GPIO export 的併發競爭、Go 的 `[]byte` 在 JSON 序列化時跟陣列格式對不上)——現在都修好了,也補上了迴歸測試。

## 怎麼用這個 MCP server

```powershell
cd mt7688-mcp-server
go test ./...      # 單元測試,不需要真的硬體
.\build.ps1         # 交叉編譯給板子用(mipsel、soft-float、靜態連結)
.\deploy.ps1        # 透過 SSH 把執行檔送到板子上
```

MCP client(例如 Claude Code)是透過每個 session 各自用 SSH 啟動一次部署好的執行檔來連線——實際指令見 [`.mcp.json`](.mcp.json),而這塊板子的 SSH 連線為什麼需要四個現代 OpenSSH 預設不會開的舊版演算法參數,說明在 [`provisioning/README.md`](provisioning/README.md)。

## 現況

還在持續進行中。MCP server 目前開放 10 個工具:GPIO(讀/寫/方向)、I2C(讀/寫)、SPI(`spi_transfer`)、PWM(`pwm_configure`/`pwm_set_enabled`/`pwm_read`)、UART(`uart_transact`)。除了 `i2c_write`(只有程式碼審查跟單元測試,手邊沒有能安全拿來測試寫入的 I2C 裝置)、`uart_transact` 的位元組級別正確性(沒有真實序列裝置或 loopback 可測——已確認呼叫本身在實機上不會出錯、真的能從硬體讀到位元組,但還沒驗證過完整的收發正確性)之外,都已經在真實硬體上做過結構性驗證。I2S、ATmega UART 橋接、ADC 還沒做成工具開放出來——ADC 尤其是 MT7688 這側完全沒有硬體路徑(實測確認沒有任何 IIO/ADC 子系統),要等 ATmega 橋接先做完。完整全貌表格見 [`docs/superpowers/specs/2026-07-11-uart-design.md`](docs/superpowers/specs/2026-07-11-uart-design.md)。

## 版權

Copyright (c) 2026 Ming-Chih Lung(台灣台南)。適用範圍請見各檔案內的宣告——第三方內容(MediaTek 的 developer's guide PDF、側載的 OpenWrt 套件)不在此列。
