<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# LinkIt Smart 7688 Duo

[English](README.md)

這是 [Seeed / MediaTek LinkIt Smart 7688 Duo](https://www.seeedstudio.com/LinkIt-Smart-7688-Duo.html)
的硬體開發筆記,以及一個給它用的 MCP(Model Context Protocol)server。這塊板子上有**兩顆獨立的處理器**:MediaTek **MT7688AN**(MIPS24KEc,跑 OpenWrt Linux)跟 Atmel **ATmega32U4**(裸機 AVR),兩者透過 UART 橋接在一起。

這個 repo 一半是「怎麼在一塊 2016 年、資源很緊的板子上真的把事情做起來」的實戰記錄,一半是一個能實際運作的工具——讓 AI coding agent(Claude Code)透過 MCP 工具直接操作板子的硬體,不用再手動開 SSH session:GPIO、I2C、SPI、PWM、UART、I2S 音訊,還有透過板上第二顆處理器提供的類比輸入。

## 這個 repo 裡有什麼

| 路徑 | 內容 |
|---|---|
| [`dev-notes.html`](dev-notes.html) | 敘事式筆記:把根檔案系統從 9.8 MB 的內建 flash 搬到 116 GiB 的 SD 卡、在 2016 年的韌體上裝現代 Python、以及一個至今沒解決的 SD 卡上動態連結執行檔 SIGILL/SIGSEGV 問題 |
| [`provisioning/`](provisioning/) | 每一個真的在板子上跑過的 shell 腳本,依照實際執行順序保留,讓整個過程在一塊全新的板子上可以重現,不用重新摸索一次。細節見 [`provisioning/README.md`](provisioning/README.md) |
| [`mt7688-mcp-server/`](mt7688-mcp-server/) | MCP server 本體——用 Go 寫、交叉編譯給板子用,開放 13 個硬體原語工具(GPIO/I2C/SPI/PWM/UART/I2S/ATmega 橋接)給 MCP client 呼叫 |
| [`atmega-firmware/`](atmega-firmware/) | 板上第二顆處理器(ATmega32U4)用的 Arduino 韌體——`adc-bridge/` 透過晶片間的 UART 講一個 Firmata 子集,讓 MT7688 這側能讀到 ATmega 的類比輸入 |
| [`docs/superpowers/specs/`](docs/superpowers/specs/) | 設計文件——記錄架構決策背後的理由,包括實作到一半才發現需要修正的部分 |
| [`docs/superpowers/plans/`](docs/superpowers/plans/) | 逐任務的實作計畫 |
| [`CLAUDE.md`](CLAUDE.md) | 給在這個 repo 裡工作的 AI coding agent 看的指引——板子架構、工具鏈、真正可用的 build/test/deploy 指令 |
| [`.claude/skills/mt7688-hardware/`](.claude/skills/mt7688-hardware/) | 一份 agent skill,教怎麼有效使用已部署的 MCP 工具——什麼情況該用哪個工具、安全限制,以及單看工具 schema 看不出來的眉角 |

## 這塊板子

| | MT7688 那側 | ATmega32U4 那側 |
|---|---|---|
| 晶片 | MediaTek MT7688AN,MIPS24KEc @ 580MHz | Microchip ATmega32U4,AVR @ 8MHz、3.3V |
| 跑的系統 | OpenWrt 15.05.1(Chaos Calmer),kernel 3.18.23 | 裸機 / Arduino 韌體 |
| 負責的事 | 網路、WiFi、USB host、SD 卡、服務 | 即時 GPIO、ADC、PWM |

兩顆晶片之間靠 UART 接在一起——「Duo」這個名字的重點就在這裡。ATmega 那側感測或控制的任何東西,都得經過這條橋接線才能連到網路,反過來也一樣。這條橋接線現在已經完整打通:`atmega-firmware/adc-bridge/` 在 ATmega 上跑一個小型 Firmata 子集,MCP server 的 `atmega_adc_read`/`atmega_bridge_raw` 工具在 Linux 這側講同一套協定——類比輸入的存取能力也是靠這條路徑提供的,因為 MT7688 本身完全沒有 ADC 硬體。

## 目前做到的事

- **Extroot 完成**:根檔案系統的可寫層從 9.8 MB 的內建 flash 搬到 116 GiB 的 SD 卡(空間大約多了一萬一千倍),重開機後仍然生效。
- **在 2016 年的韌體上跑現代 Python**:靠 Entware 的 `mipselsf-k3.4` target,自帶 glibc,跟系統的 uClibc 完全隔離——但有一個已經記錄下來、還沒解決的穩定性問題(見 `dev-notes.html`)。
- **抓到並修好一個真實的網路橋接迴圈**:WiFi 上行介面不小心跟有線網路埠橋接在同一個 L2 網段、又沒開 STP——這是直接在板子上現場診斷出來的,不是用猜的。
- **一個真的能用的 MCP server**,已經部署到真實硬體上驗證過:13 個工具涵蓋 GPIO/I2C/SPI/PWM/UART/I2S/ATmega 橋接,寫死的安全黑名單(console UART、SD/extroot 控制線、ATmega 橋接 UART、ISP 相關腳位)在碰硬體之前一定先檢查,而且抓到好幾個只有在真實測試或 code review 時才會浮現的 bug(GPIO export 的併發競爭、Go 的 `[]byte` 在 JSON 序列化時跟陣列格式對不上、一個可能讓 ATmega 橋接卡在協定中間卻被靜默吞掉的錯誤)——現在都修好了,也補上了迴歸測試。
- **第二顆處理器完整打通**:ATmega32U4 跑 Arduino 韌體,透過它自己專屬的 UART(在 Linux 這側實測確認是 `/dev/ttyS0`,跟 `uart_transact` 用的通用 UART是不同的實體埠)講一個精簡版 Firmata 協定,讓 MCP server 擁有 MT7688 晶片本身沒有的真正類比輸入能力。

## 怎麼用這個 MCP server

```powershell
cd mt7688-mcp-server
go test ./...      # 單元測試,不需要真的硬體
.\build.ps1         # 交叉編譯給板子用(mipsel、soft-float、靜態連結)
.\deploy.ps1        # 透過 SSH 把執行檔送到板子上
```

MCP client(例如 Claude Code)是透過每個 session 各自用 SSH 啟動一次部署好的執行檔來連線——實際指令見 [`.mcp.json`](.mcp.json),而這塊板子的 SSH 連線為什麼需要四個現代 OpenSSH 預設不會開的舊版演算法參數,說明在 [`provisioning/README.md`](provisioning/README.md)。

## 現況

原本規劃的硬體覆蓋路線圖(GPIO/I2C → SPI/PWM → UART → I2S → ATmega 橋接 → ADC)已經全部完成。MCP server 目前開放 13 個工具:GPIO(讀/寫/方向)、I2C(讀/寫)、SPI(`spi_transfer`)、PWM(`pwm_configure`/`pwm_set_enabled`/`pwm_read`)、UART(`uart_transact`)、I2S(`i2s_play`)、以及 ATmega32U4 橋接(`atmega_adc_read`、`atmega_bridge_raw`)。除了 `i2c_write`(只有程式碼審查跟單元測試,手邊沒有能安全拿來測試寫入的 I2C 裝置)、`uart_transact` 的位元組級別正確性(沒有真實序列裝置或 loopback 可測)、`i2s_play` 的實際聽感(驗證當時沒接喇叭,只確認呼叫本身不會出錯,沒辦法確認聲音品質)之外,都已經在真實硬體上做過結構性驗證。I2S 的錄音**確認不可用**,不只是還沒做——板子的錄音路徑有可重現的驅動層 bug 會讓音效裝置卡死(連 `SIGKILL` 都殺不掉,要重開機才能解),設計階段就查到並記錄下來,後來也試過加大 buffer 當作變通方法但同樣失敗,詳見 [`docs/superpowers/specs/2026-07-11-i2s-design.md`](docs/superpowers/specs/2026-07-11-i2s-design.md) 的 Investigation 段落。ATmega UART 橋接經過四個階段(工具鏈建置、Firmata 協定設計、ATmega 韌體、MT7688 側 decoder + MCP 工具——見 [`docs/superpowers/specs/2026-07-11-atmega-phase-b-wire-protocol.md`](docs/superpowers/specs/2026-07-11-atmega-phase-b-wire-protocol.md)、[`docs/superpowers/specs/2026-07-11-atmega-phase-d-mcp-tools.md`](docs/superpowers/specs/2026-07-11-atmega-phase-d-mcp-tools.md) 及對應的 plan 文件)已經完整打通並在真實硬體上驗證過,包含直接解讀 Firmata 協定原始位元組的驗證。目前還有一個未解決的正確性問題記在 `CLAUDE.md` 裡:GPIO 黑名單裡「ATmega ISP reset」那筆記錄的腳位號碼可能是錯的(真實證據指向 pin 3,黑名單目前寫的是 pin 38)——已標記但還沒修。現在 ATmega 橋接已經上線,這件事更值得盡快處理:如果有 MCP 呼叫者以為 GPIO 3 只是個 LED 而去切換它,會在韌體運作中把 ATmega 重置,因為現在真的有韌體、也真的有 MCP 工具依賴它,不再是「反正沒東西可以被打斷」的狀態。

## 版權

Copyright (c) 2026 Ming-Chih Lung(台灣台南)。適用範圍請見各檔案內的宣告——第三方內容(MediaTek 的 developer's guide PDF、側載的 OpenWrt 套件)不在此列。
