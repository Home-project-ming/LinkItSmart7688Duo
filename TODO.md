<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# TODO

> 專案的待辦與已知缺口,依價值排序。原本規劃的硬體覆蓋路線圖
> (GPIO/I2C → SPI/PWM → UART → I2S → ATmega 橋接 → ADC)已全部完成
> ——這裡列的是路線圖之外還掛著的事。

## 🥇 最值得投入:squashfs 實驗(修 SD 卡執行檔隨機崩潰)

- [ ] **把 `/opt`(Entware)打包成 squashfs 映像檔放在 SD 上、loop-mount 使用,驗證是否根絕 SIGILL/SIGSEGV。**

**問題**:`/opt` 在 SD 卡上,任何動態連結執行檔的頁面被逐出 page cache
後、從 SD 重新 fault 回來,就會隨機 SIGILL/SIGSEGV/SIGBUS
(`dev-notes.html` 的「SD 執行不穩」章節有完整證據與已排除清單)。
後果:板上的 Python 不能放心當常駐服務,`rc.local` 刻意沒有加自啟。

**為什麼 squashfs 有機會**:squashfs 的頁面是 CPU 解壓縮後寫入的,
不是 DMA 直接填入被 mmap 的頁面——這剛好解釋為什麼內建 flash 上的
squashfs(`/rom`)從來沒出過事。

**為什麼值得**:若成功,「板上跑 Python 常駐服務」從不可靠變可靠,
`applications.html` 第 3 類(免動手的常駐網路服務)一大半點子都直接受惠。

**驗收方式**:沿用 dev-notes 記錄的測試法——`drop_caches` 後冷啟動
10 次;現行 ext4 直讀為對照組(歷史紀錄 0/10),squashfs loop-mount
實驗組目標 10/10。

**根治備案**:升級到 kernel ≥ 5.x 的現代 OpenWrt(工程量大、風險另議)。

## 🔌 等硬體到位才能補的驗證缺口

- [ ] `i2c_write`:目前只有 code review + 單元測試——需要一顆可安全寫入的
  I2C 裝置(例如 BME280)實測。
- [ ] `uart_transact` 位元組級 round-trip:TX(pin 45)–RX(pin 46)接一條
  loopback 跳線即可測——等願意動硬體的時候順手做。
- [ ] `i2s_play` 實際聽感:只驗證過呼叫不報錯;接上喇叭確認聲音乾淨。

## 📋 Backlog(從未被需要,不欠)

- [ ] MIPS 原生 C 交叉編譯工具鏈——目前 Go 自帶的交叉編譯就夠;若有純 C
  需求再建,並回填 `CLAUDE.md` 的 TODO。
- [ ] ATmega 的 USB 有線上傳流程(`arduino-cli upload -p`)——網路 ISP
  燒錄已夠用,需要時再驗證。

## 🚫 已結案,不要再開

- **I2S 錄音**:驅動層死巷——capture 一啟動就卡死(SIGKILL 殺不掉、須
  重開機),codec 電源/路由確認過、buffer/period 加大也試過,同樣失敗。
  詳見 `docs/superpowers/specs/2026-07-11-i2s-design.md` 的 Investigation
  段落。
- **GPIO 黑名單 pin 3/38 之謎**:2026-07-12 已修正並實機驗證
  (pin 3 = I2S + ATmega ISP reset;pin 38 = Wi-Fi Reset 按鈕)。
