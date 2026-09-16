package server

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Server 自身 SQLite 备份（见 docs/guide/server-backup-design.md）：
// VACUUM INTO 在线产生紧凑副本到 <db目录>/server-backups/，目录即事实源
// （不建 DB 表）。恢复 = 停服替换文件（D8）。

// Setting 键：备份间隔（小时）与保留天数（每轮读取免缓存）
const (
	ServerBackupIntervalSettingKey  = "server_backup.interval_hours"
	ServerBackupRetentionSettingKey = "server_backup.retention_days"
)

const (
	serverBackupDefaultIntervalHours = 24
	serverBackupMaxIntervalHours     = 168
	serverBackupDefaultRetentionDays = 7
	serverBackupMaxRetentionDays     = 365

	// 文件名严格模式：cockpit-YYYYMMDD-HHMMSS.db（D7，防穿越/任意删除）
	serverBackupNameRe = `^cockpit-\d{8}-\d{6}\.db$`
)

// serverBackupLoop 醒来节奏。包级变量仅为测试可注入，默认值即生产取值。
var serverBackupTick = time.Hour

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
	return name, nil
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
