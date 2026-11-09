// ============ 定时任务（Crontab 管理） ============

// cron.status：概览
export interface CronStatus {
  user: string
  cockpitCount: number
  externalCount: number
}

// cockpit 名下的一个定时任务（next_run 为下次触发预览 unix 秒，
// disabled/@reboot/解析失败为 0；见 docs/guide/cron-design.md M2）
export interface CronJob {
  name: string
  schedule: string
  command: string
  enabled: boolean
  next_run?: number
}

// cron.jobs 返回：cockpit 任务列表 + 外部条目原文（只读）
export interface CronJobsResult {
  jobs: CronJob[]
  external: string
}

// cron.users 枚举的一个系统用户（cron-design.md M4 D22：不做 nologin 过滤，
// 服务用户恰是 crontab 主人）
export interface CronUserEntry {
  name: string
  uid: number
  shell?: string
}

// 一个 systemd timer unit 的只读快照（cron-design.md M3：日程为
// OnCalendar/OnBootUSec 原文不做语义解析；时间为 unix 秒，缺失/不可解析为 0；
// 模板单元等 show 失败的仅 unit 名，其余字段空）
export interface SystemdTimer {
  unit: string
  description: string
  schedule: string
  state: string
  unitFileState: string
  last_trigger: number
  next_run: number
}
