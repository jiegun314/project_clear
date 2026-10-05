import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { errorText } from './errors.ts';

// Wails rejects a bound call with the Go error's text, a webview-side throw
// arrives as a real Error, and anything else is handed through as it was. All
// three shapes reach the same notice, so they have to render as a sentence.
describe('errorText', () => {
  it('passes a Go error string through', () => {
    assert.equal(errorText('没有可导出的数据，请先导入文件'), '没有可导出的数据，请先导入文件');
  });

  it('uses the message of a thrown Error', () => {
    assert.equal(errorText(new Error('boom')), 'boom');
  });

  it('stringifies anything else rather than showing [object Object]', () => {
    assert.equal(errorText(undefined), 'undefined');
    assert.equal(errorText(null), 'null');
    assert.equal(errorText(42), '42');
  });
});
