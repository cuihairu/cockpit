<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { withBase } from 'vitepress'

// 首页 hero 下方「界面预览」走马灯：纯 Vue + CSS 轮播（无第三方依赖）。
// 图片来自 docs/public/screenshots/（本地起 dashboard 用 chromium 真实截取），
// 路径须经 withBase 以兼容 GitHub Pages 子路径部署（base: /cockpit/）。
const shots = [
  { src: '/screenshots/dashboard.png', alt: '总览：资源统计卡、健康度与 Agent 列表（离线置底弱化并标注离线时长）' },
  { src: '/screenshots/agents-cleanup.png', alt: 'Agent 管理：一键清理离线（24 小时 / 3 天 / 7 天三档阈值二次确认）' },
  { src: '/screenshots/dns.png', alt: 'DNS 记录管理：按 zone 增删改查与批量导入 / 导出' },
  { src: '/screenshots/audit.png', alt: '审计日志：动作 / 资源 / 结果多维检索' },
  { src: '/screenshots/workbench.png', alt: '工作台：容器编排与快速操作入口' },
]

const idx = ref(0)
let timer: ReturnType<typeof setInterval> | undefined

const go = (i: number) => {
  idx.value = (i + shots.length) % shots.length
}
const next = () => go(idx.value + 1)
const prev = () => go(idx.value - 1)

onMounted(() => {
  timer = setInterval(next, 5000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <section class="shots">
    <h2 class="shots-title">界面预览</h2>
    <div class="shots-stage">
      <button class="shots-nav" aria-label="上一张" @click="prev()">‹</button>
      <div class="shots-frame">
        <img
          v-for="(s, i) in shots"
          :key="s.src"
          class="shots-img"
          :class="{ active: i === idx }"
          :src="withBase(s.src)"
          :alt="s.alt"
          :loading="i === idx ? 'eager' : 'lazy'"
        />
        <p class="shots-caption">{{ shots[idx].alt }}</p>
      </div>
      <button class="shots-nav" aria-label="下一张" @click="next()">›</button>
    </div>
    <div class="shots-dots" role="tablist" aria-label="界面预览分页">
      <button
        v-for="(s, i) in shots"
        :key="s.src"
        class="shots-dot"
        :class="{ active: i === idx }"
        :aria-label="`第 ${i + 1} 张`"
        @click="go(i)"
      />
    </div>
  </section>
</template>

<style scoped>
.shots {
  margin: 28px auto 8px;
  padding: 0 24px;
  max-width: 1152px;
}
.shots-title {
  font-size: 20px;
  font-weight: 600;
  margin: 0 0 12px;
  text-align: center;
}
.shots-stage {
  display: flex;
  align-items: center;
  gap: 8px;
}
.shots-frame {
  position: relative;
  flex: 1;
  aspect-ratio: 16 / 9;
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid var(--vp-c-border);
  background: var(--vp-c-bg-soft);
  box-shadow: var(--vp-shadow-2);
}
.shots-img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
  opacity: 0;
  transition: opacity 0.6s ease;
}
.shots-img.active {
  opacity: 1;
}
.shots-caption {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  margin: 0;
  padding: 6px 12px;
  font-size: 13px;
  text-align: center;
  color: var(--vp-c-text-2);
  background: color-mix(in srgb, var(--vp-c-bg) 78%, transparent);
}
.shots-nav {
  flex: none;
  width: 34px;
  height: 34px;
  border-radius: 50%;
  border: 1px solid var(--vp-c-border);
  background: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
}
.shots-nav:hover {
  background: var(--vp-c-bg-soft);
}
.shots-dots {
  display: flex;
  justify-content: center;
  gap: 8px;
  margin-top: 10px;
}
.shots-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  border: none;
  padding: 0;
  cursor: pointer;
  background: var(--vp-c-border);
}
.shots-dot.active {
  background: var(--vp-c-brand-1);
}
@media (max-width: 640px) {
  .shots-nav {
    display: none;
  }
}
</style>
