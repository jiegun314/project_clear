/**
 * 后台任务的进度显示。
 *
 * 后端在干活的过程中不断发 `task:progress` 事件，最后一个往往是 "完成 N/N"；
 * Wails 的事件是异步投递的，这条收尾事件可能比 await 的回调更晚到达，于是状态栏
 * 就被改回"红色转圈"并且一直停在那里。这里把状态拆开：任务进行中显示红色转圈，
 * 成功收尾后由前端置 `finished`，显示绿色对勾。
 */

export interface TaskProgress {
  stage: string;
  done: number;
  total: number;
  /** 任务已经成功收尾（前端置位），状态栏显示绿色完成标记 */
  finished?: boolean;
}

/**
 * acceptProgress 决定一条后端事件要不要采纳：任务已经结束（或者压根没在跑）时
 * 直接丢掉，避免迟到的收尾事件把状态栏改回运行中。
 */
export function acceptProgress(
  taskRunning: boolean,
  p: TaskProgress | null | undefined,
): TaskProgress | null {
  if (!taskRunning || !p) return null;
  return p;
}

/** finishedProgress 是任务成功后的收尾状态。 */
export function finishedProgress(): TaskProgress {
  return { stage: '完成', done: 1, total: 1, finished: true };
}

/** progressText 是状态栏上跟着图标的那段文字。 */
export function progressText(p: TaskProgress): string {
  if (p.finished) return p.stage;
  return p.total > 1 ? `${p.stage} ${p.done}/${p.total}` : p.stage;
}
