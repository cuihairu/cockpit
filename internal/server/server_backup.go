package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/notification"
)

// Server 自身 SQLite 备份（见 docs/guide/server-backup-design.md）：
// VACUUM INTO 在线产生紧凑副本到 <db目录>/server-backups/，目录即事实源
// （不建 DB 表）。恢复 = 停服替换文件（D8）。
// M2：本地备份成功后 rclone copy 推送异地（remote:path，凭据只在
// server 主机 rclone.conf），推送失败不影响本地成果（D14/D16）。

// Setting 键：备份间隔（小时）与保留天数（每轮读取免缓存）
const (
	ServerBackupIntervalSettingKey   = "server_backup.interval_hours"
	ServerBackupRetentionSettingKey  = "server_backup.retention_days"
	ServerBackupRemoteDestSettingKey = "server_backup.remote_dest"
)

const (
	serverBackupDefaultIntervalHours = 24
	serverBackupMaxIntervalHours     = 168
	serverBackupDefaultRetentionDays = 7
	serverBackupMaxRetentionDays     = 365

	// 异地目标上限与形态校验（M2 D13，与 agent 侧 backupRemoteDestRe 同源：
	// remote 名不以 - 开头根除 flag 混淆）
	serverBackupRemoteDestMaxLen = 512
	serverBackupRemoteDestRe     = `^[A-Za-z0-9][A-Za-z0-9._-]*:[^\x00-\x20\x7f]+$`

	// 文件名严格模式：cockpit-YYYYMMDD-HHMMSS.db（D7，防穿越/任意删除）
	serverBackupNameRe = `^cockpit-\d{8}-\d{6}\.db$`
)

// serverBackupLoop 醒来节奏。包级变量仅为测试可注入，默认值即生产取值。
var serverBackupTick = time.Hour

// rclone 可执行文件与推送超时（server 备份与录制归档共用，recording M2
// D15）。包级变量仅为测试可注入，默认值即生产取值。读写一律持
// serverRcloneMu：录制归档是异步 goroutine，与测试清理的注入还原并发，
// 裸读写即数据竞争（CI -race 捕获两例）。
var (
	serverRcloneMu            sync.RWMutex
	serverRcloneBin           = "rclone"
	serverBackupRemoteMaxWait = 5 * time.Minute
)

// serverRcloneConfig 快照当前 rclone 注入值（可执行文件, 推送超时）
func serverRcloneConfig() (string, time.Duration) {
	serverRcloneMu.RLock()
	defer serverRcloneMu.RUnlock()
	return serverRcloneBin, serverBackupRemoteMaxWait
}

// rcloneCopyLocalFile rclone copy 推送单个本地文件到远端（server 备份 M2
// D11 与录制归档 M2 D15 共用）。超时整杀 + WaitDelay 强断管道防孤儿孙进程
// 拖住 Wait；失败错误含 stderr 摘要（exit status 无解释力）。
func rcloneCopyLocalFile(local, remoteDest string) error {
	bin, maxWait := serverRcloneConfig()
	ctx, cancel := context.WithTimeout(context.Background(), maxWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "copy", "--transfers", "2", local, remoteDest)
	// ctx 取消只杀 rclone 本体；继承 stdout 的孙进程（若有）会占住管道，
	// WaitDelay 到期强断，Wait 不被孤儿拖住（rclone 官方单二进制，无此链）
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if summary := strings.TrimSpace(string(out)); summary != "" {
		if len(summary) > 2048 {
			summary = summary[:2048] + "…"
		}
		log.Printf("[remote] %s", summary)
	}
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if len(detail) > 512 {
		detail = detail[:512]
	}
	if detail != "" {
		return fmt.Errorf("rclone copy: %v: %s", err, detail)
	}
	return fmt.Errorf("rclone copy: %v", err)
}

// serverBackupRemoteDest 读异地目标 Setting；空=关闭。脏值（写入口之后
// 被手改）视为未配置并记日志，不阻塞本地备份（D13 读宽松）。
func (s *Server) serverBackupRemoteDest() string {
	v, err := s.db.GetSetting(ServerBackupRemoteDestSettingKey)
	if err != nil || v == "" {
		return ""
	}
	if len(v) > serverBackupRemoteDestMaxLen ||
		!regexp.MustCompile(serverBackupRemoteDestRe).MatchString(v) {
		log.Printf("Server backup remote_dest invalid, treating as unset: %q", v)
		return ""
	}
	return v
}

// serverBackupIntervalHours 备份间隔；0=关闭；非法回默认
func (s *Server) serverBackupIntervalHours() int {
	v, err := s.db.GetSetting(ServerBackupIntervalSettingKey)
	if err != nil || v == "" {
		return serverBackupDefaultIntervalHours
	}
	n, convErr := strconv.Atoi(v)
	if convErr != nil || n < 0 || n > serverBackupMaxIntervalHours {
		return serverBackupDefaultIntervalHours
	}
	return n
}

// serverBackupRetentionDays 保留天数；0=永久；非法回默认
func (s *Server) serverBackupRetentionDays() int {
	v, err := s.db.GetSetting(ServerBackupRetentionSettingKey)
	if err != nil || v == "" {
		return serverBackupDefaultRetentionDays
	}
	n, convErr := strconv.Atoi(v)
	if convErr != nil || n < 0 || n > serverBackupMaxRetentionDays {
		return serverBackupDefaultRetentionDays
	}
	return n
}

// serverBackupDir 备份目录：数据库同目录下 server-backups/
func (s *Server) serverBackupDir() string {
	dbPath := "./data/cockpit.db"
	if s.cfg != nil && s.cfg.Database != nil && s.cfg.Database.Path != "" {
		dbPath = s.cfg.Database.Path
	}
	return filepath.Join(filepath.Dir(dbPath), "server-backups")
}

// serverBackupFile 单个备份文件的视图（列表 API 返回结构）
type serverBackupFile struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

// isValidServerBackupName 校验备份文件名（严格模式，D7）
func isValidServerBackupName(name string) bool {
	ok, err := regexp.MatchString(serverBackupNameRe, name)
	return err == nil && ok
}

// listServerBackups 扫描目录得到备份列表（按文件名倒序 = 时间倒序）
func (s *Server) listServerBackups() ([]serverBackupFile, error) {
	entries, err := os.ReadDir(s.serverBackupDir())
	if os.IsNotExist(err) {
		return []serverBackupFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []serverBackupFile
	for _, e := range entries {
		if e.IsDir() || !isValidServerBackupName(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		list = append(list, serverBackupFile{Name: e.Name(), Size: info.Size(), ModTime: info.ModTime()})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name > list[j].Name })
	return list, nil
}

// latestServerBackupAt 最近一次备份的时间（无备份返回零值）
func (s *Server) latestServerBackupAt() time.Time {
	list, err := s.listServerBackups()
	if err != nil || len(list) == 0 {
		return time.Time{}
	}
	// 文件名即时间戳（UTC 本地无关，仅作间隔对比）
	name := strings.TrimSuffix(strings.TrimPrefix(list[0].Name, "cockpit-"), ".db")
	t, err := time.ParseInLocation("20060102-150405", name, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// runServerBackup 执行一次 VACUUM INTO 备份，返回文件名
func (s *Server) runServerBackup() (string, error) {
	dir := s.serverBackupDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create server-backups dir: %w", err)
	}
	name := "cockpit-" + time.Now().Format("20060102-150405") + ".db"
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("backup already exists this second: %s", name)
	}
	if err := s.db.VacuumInto(path); err != nil {
		os.Remove(path) // 半成品不留
		return "", fmt.Errorf("vacuum into: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		log.Printf("Server backup chmod %s failed: %v", name, err)
	}
	log.Printf("Server DB backup created: %s", name)
	s.pushServerBackupRemote(name)
	return name, nil
}

// pushServerBackupRemote 本地备份成功后的异地推送（M2 D14：失败不影响本地
// 成果与调用方返回）。remote_dest 未配置时静默跳过。
func (s *Server) pushServerBackupRemote(name string) {
	remoteDest := s.serverBackupRemoteDest()
	if remoteDest == "" {
		return
	}
	local := filepath.Join(s.serverBackupDir(), name)
	log.Printf("[remote] rclone copy %s → %s", name, remoteDest)
	if err := rcloneCopyLocalFile(local, remoteDest); err != nil {
		log.Printf("[remote] server backup push failed: %v", err)
		s.notifyServerBackupRemoteFailed(name, err.Error())
		return
	}
	log.Printf("[remote] server backup pushed: %s", name)
}

// notifyServerBackupRemoteFailed 推送失败通知（M2 D16：本地已成功，仅远端
// 链路断；走独立事件白名单，notifier 未启用时 SendNonBlocking 自身跳过）。
func (s *Server) notifyServerBackupRemoteFailed(name, errMsg string) {
	if s.notifier == nil {
		return
	}
	s.notifier.SendNonBlocking(&notification.Notification{
		EventType:    notification.ServerBackupRemoteFailed,
		Title:        "面板数据库异地推送失败",
		Message:      fmt.Sprintf("%s: %s", name, errMsg),
		Level:        "error",
		ResourceType: "server_backup",
		ResourceID:   name,
		Time:         time.Now(),
	})
}

// rcloneAvailable 异地推送执行前提的实时探测（server 备份 M2 D18：后装
// rclone 免重启）
func rcloneAvailable() bool {
	bin, _ := serverRcloneConfig()
	_, err := exec.LookPath(bin)
	return err == nil
}

// cleanupServerBackups 按保留天数清理过期备份（0=永久）
func (s *Server) cleanupServerBackups() {
	days := s.serverBackupRetentionDays()
	if days <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	list, err := s.listServerBackups()
	if err != nil {
		return
	}
	removed := 0
	for _, b := range list {
		if b.ModTime.Before(cutoff) {
			if err := os.Remove(filepath.Join(s.serverBackupDir(), b.Name)); err == nil {
				removed++
			}
		}
	}
	if removed > 0 {
		log.Printf("Cleaned up %d expired server backups", removed)
	}
}

// serverBackupLoop 定时备份循环（D3：每小时醒，间隔可动态改）
func (s *Server) serverBackupLoop() {
	ticker := time.NewTicker(serverBackupTick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if s.serverBackupIntervalHours() <= 0 {
				continue // 关闭
			}
			last := s.latestServerBackupAt()
			if !last.IsZero() && time.Since(last) < time.Duration(s.serverBackupIntervalHours())*time.Hour {
				continue
			}
			if _, err := s.runServerBackup(); err != nil {
				log.Printf("Server DB backup failed: %v", err)
			}
			s.cleanupServerBackups()
		case <-s.ctx.Done():
			return
		}
	}
}
