<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# TODO

> 專案的待辦與已知缺口,依價值排序。原本規劃的硬體覆蓋路線圖
> (GPIO/I2C → SPI/PWM → UART → I2S → ATmega 橋接 → ADC)已全部完成
> ——這裡列的是路線圖之外還掛著的事。

## ✅ 已完成:squashfs /opt 上線化(2026-07-14)

- [x] 開機自動 `losetup` + mount `/overlay/opt.sqfs`,把 `/opt` 指向唯讀
  映像——Entware 服務從此可以安全開機自啟。
- [x] 設計「改動 /opt 之後重建映像」的流程:`opkg` 裝/更新套件 → 重跑
  `provisioning/05-squashfs-opt/03_build_image.sh` → remount。日常唯讀,
  更新時才寫。
- [x] 重開機驗證自動掛載生效後,解除 dev-notes 裡「不要把 /opt 放進
  開機自啟服務」的禁令(`rc.unslung` 可以進 `/etc/rc.local` 了)。

**結果:成功,實機重開機驗證通過。** 新增 `provisioning/05-squashfs-opt/`
兩支腳本:`06_install_boot_hook.sh`(冪等安裝開機掛載鉤子到
`/etc/rc.local`,失敗不影響開機/不影響 SSH)、`07_opt_maintenance.sh`
(`unlock`/`rebuild` 兩個子命令,對應「解掛露出可寫目錄 → opkg 操作 →
重建映像+重新掛載」的維護流程)。實際對板子下 `reboot` 後確認:
`/opt` 無人值守自動掛載成 `squashfs(ro)`、寫入測試回報
`Read-only file system`、冷快取下 `grep --version`/`python3 --version`
皆正常執行,不再重現 03-diagnostics 的 SIGILL/SIGSEGV。設計與實作細節見
`docs/superpowers/specs/2026-07-14-squashfs-opt-boot-mount-design.md`
與 `docs/superpowers/plans/2026-07-14-squashfs-opt-boot-mount.md`。
dev-notes.html 的開機自啟禁令已同步解除。

## ✅ 已完成:squashfs 實驗(2026-07-12)——SD 執行檔隨機崩潰找到繞道

- [x] 把 `/opt` 打包成 squashfs 映像放 SD、loop-mount,驗證是否根絕 SIGILL/SIGSEGV。

**結果:成功,且數據乾淨利落。** 同一張 SD、同樣的檔案、同樣的
`drop_caches` 冷快取條件:ext4 直讀對照組 grep **0/10**、python3
**0/10**(完全重現歷史數據);squashfs loop-mount 實驗組 grep
**10/10**、python3 **10/10**;加碼 soak(grep/python3 各 20 次 +
`PYTHONHOME` 指進映像的真實 Python 運算 10 次)**70/70 全過**。
同時佐證 I-cache/DMA 不一致理論——squashfs 頁面由 CPU 解壓寫入,
不走 DMA 直填 mmap 頁面。

**細節**:kernel 的 squashfs 只編了 xz 解壓(gzip 映像會被拒),映像
27.4MB(原 67.5MB);需側載 `kmod-loop`+`losetup`(原廠 kernel 沒有
loop device);mksquashfs 要從 tmpfs 執行(它自己就是會冷快取崩潰的
SD 上 glibc 執行檔)。腳本與側載 ipk 都在
`provisioning/05-squashfs-opt/`。目前的 loop mount 重開機即消失——
上線化見上方新的第一名。

**根治備案**(不急):升級到 kernel ≥ 5.x 的現代 OpenWrt。

## 🎯 進行中:XCS20 FPGA slave-serial 下載(2026-07-30 設計完成)

- [ ] 依設計文件實作與 bring-up:
  `docs/superpowers/specs/2026-07-30-xcs20-slave-serial-download-design.md`
  ——用 7688 經 74AHCT125 轉壓接目標板 DB25,GPIO bit-bang slave serial,
  拔 EPROM 下載(實機驗證過此模式可行,綠燈=成功)。bitstream 格式已
  三重驗證(MSB-first、header 可變長、promgen=逐 byte 位元反轉)。
- [ ] 待辦前置:焊 AHCT125 轉接板(BOM 在設計文件 §3)、確認 7688 排針
  可用腳位(§5)、板子回線上後補唯讀檢查(§10)。

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
