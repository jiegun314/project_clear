import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { acceptProgress, finishedProgress, progressText } from './taskProgress.ts';

describe('acceptProgress', () => {
  const late = { stage: '完成', done: 2, total: 2 };

  it('keeps events while the task is running', () => {
    assert.deepEqual(acceptProgress(true, late), late);
  });

  it('drops the late "完成 N/N" event once the task has settled', () => {
    // 这正是导出后状态栏停在红色"运行中"的原因：收尾事件比 await 回调晚到。
    assert.equal(acceptProgress(false, late), null);
  });

  it('ignores an empty payload', () => {
    assert.equal(acceptProgress(true, null), null);
    assert.equal(acceptProgress(true, undefined), null);
  });
});

describe('finishedProgress', () => {
  it('marks the task as finished so the bar can paint it green', () => {
    const p = finishedProgress();
    assert.equal(p.finished, true);
    assert.equal(progressText(p), '完成');
  });
});

describe('progressText', () => {
  it('shows the counter only while more than one step is expected', () => {
    assert.equal(progressText({ stage: '导出', done: 1, total: 2 }), '导出 1/2');
    assert.equal(progressText({ stage: '整合入库', done: 0, total: 1 }), '整合入库');
    assert.equal(progressText({ stage: '准备中', done: 0, total: 0 }), '准备中');
  });
});
