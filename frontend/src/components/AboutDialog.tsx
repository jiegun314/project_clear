import { useEffect, useRef, useState } from 'react';
import { Modal, Descriptions, Typography, Space } from 'antd';
import { JNJ } from '../theme/jnj';
import { api } from '../services/api';
import type { AppInfo } from '../types';

// 关于窗口里的小机关：5 秒内在项目图标上点 8 次。
// 这段逻辑有意保持安静——图标不显示手型光标、也没有任何悬停反馈。
const EGG_CLICKS = 8;
const EGG_WINDOW_MS = 5000;
const EGG_IMAGE = '/easteregg/easter_egg.png';
const EGG_AUDIO = '/easteregg/Small-dog-barking-sound-effect.mp3';
// 内容区做成 331×331 的正方形，图片 310×310 在正中（四周各留 10.5px）；
// antd 的标题栏在内容区上方，所以整个弹窗外框会比 331 更高。
const EGG_CONTENT_SIZE = 331;
const EGG_IMAGE_SIZE = 310;

export interface AboutDialogProps {
  open: boolean;
  onClose: () => void;
}

export function AboutDialog({ open, onClose }: AboutDialogProps) {
  const [info, setInfo] = useState<AppInfo | null>(null);
  const [eggOpen, setEggOpen] = useState(false);
  const eggClicks = useRef<number[]>([]);
  const eggAudio = useRef<HTMLAudioElement | null>(null);
  // 彩蛋是靠"点图标"触发的，紧接着我们会主动关掉关于窗口；这个标记让
  // "关于窗口关闭就收掉彩蛋"的清理逻辑跳过这一次有意为之的关闭。
  const eggTookOver = useRef(false);

  const stopEggAudio = () => {
    const audio = eggAudio.current;
    if (!audio) return;
    audio.pause();
    audio.currentTime = 0;
    eggAudio.current = null;
  };

  // 关掉关于窗口或组件卸载时都别留下声音。
  useEffect(() => {
    if (open) return;
    if (eggTookOver.current) return;
    setEggOpen(false);
    stopEggAudio();
  }, [open]);

  useEffect(() => stopEggAudio, []);

  useEffect(() => {
    if (!open) return;
    api.getAppInfo().then(setInfo).catch(() => setInfo(null));
  }, [open]);

  const closeEgg = () => {
    eggTookOver.current = false;
    setEggOpen(false);
    stopEggAudio();
  };

  const onLogoClick = () => {
    const now = Date.now();
    const recent = eggClicks.current.filter((t) => now - t < EGG_WINDOW_MS);
    recent.push(now);
    eggClicks.current = recent;
    if (recent.length < EGG_CLICKS) return;

    eggClicks.current = [];
    stopEggAudio();
    // 声音必须在点击回调里就起播：WKWebView 只认用户手势，而弹窗要等下一帧才挂载。
    const audio = new Audio(EGG_AUDIO);
    audio.loop = true;
    audio.volume = 0.75;
    void audio.play().catch(() => undefined);
    eggAudio.current = audio;
    // 彩蛋接管屏幕：关于窗口同时关掉。
    eggTookOver.current = true;
    setEggOpen(true);
    onClose();
  };

  return (
    <>
      <Modal open={open} onCancel={onClose} onOk={onClose} okText="关闭" cancelButtonProps={{ style: { display: 'none' } }} width={520} footer={null}>
        <div style={{ textAlign: 'center', padding: '8px 0 4px' }}>
          <img
            src="/clear.png"
            alt="CLEAR"
            draggable={false}
            onClick={onLogoClick}
            onError={(e) => {
              (e.currentTarget as HTMLImageElement).style.display = 'none';
            }}
            // 光标保持默认箭头：不要给出"这里可以点"的暗示。
            style={{ width: 84, height: 84, objectFit: 'contain', cursor: 'default' }}
          />
          <div style={{ fontSize: 26, fontWeight: 700, color: JNJ.red, letterSpacing: 4, marginTop: 6 }}>
            CLEAR
          </div>
          <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 2 }}>
            {info?.fullName ?? 'Consolidation & Loading of Enterprise Analytics for Replenishment'}
          </Typography.Text>
          <div style={{ marginTop: 6, color: JNJ.text, fontSize: 13 }}>MPS 数据整合与补货分析平台</div>
          <div style={{ marginTop: 2, color: JNJ.textMuted, fontSize: 12 }}>版本 {info?.version ?? '—'}</div>
        </div>

        <Descriptions
          column={1}
          size="small"
          bordered
          style={{ marginTop: 16 }}
          labelStyle={{ width: 92, color: JNJ.text, fontSize: 12 }}
          contentStyle={{ fontSize: 12 }}
          items={[
            { key: 'd', label: '数据目录', children: info?.dataDir ?? '—' },
            { key: 'c', label: '参数文件', children: info?.configPath ?? '—' },
            { key: 'db', label: '数据库', children: info?.database ?? '—' },
            { key: 'g', label: '运行环境', children: `${info?.goVersion ?? ''} · ${info?.platform ?? ''}` },
          ]}
        />
        <Space direction="vertical" size={2} style={{ marginTop: 12, width: '100%' }}>
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>
            技术方案：Go + Wails + Excelize，前端 React + Ant Design。
          </Typography.Text>
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>
            每周整合一次，永久数据按周码命名（如 2639），重复整合将覆盖原表。
          </Typography.Text>
        </Space>
      </Modal>

      <Modal
        open={eggOpen}
        onCancel={closeEgg}
        footer={null}
        centered
        // 点遮罩不关闭：触发它的那一下点击就落在刚出现的遮罩上，默认行为会把它自己关掉；
        // 关这个弹窗只认右上角的按钮。
        maskClosable={false}
        width={EGG_CONTENT_SIZE}
        title="🐶Puppy Approved!🐾"
        // 面板内边距在 style.css 里按这个类名归零，内容区才能是整 331×331。
        rootClassName="egg-modal"
        styles={{
          // 标题栏用淡灰底，和白色主体区分开。
          header: { padding: '12px 16px', marginBottom: 0, background: JNJ.fill },
          body: {
            height: EGG_CONTENT_SIZE,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            // 内边距交给 flex 居中处理，内容区才能保持 331×331。
            padding: 0,
          },
        }}
      >
        <img
          src={EGG_IMAGE}
          alt=""
          draggable={false}
          style={{
            width: EGG_IMAGE_SIZE,
            height: EGG_IMAGE_SIZE,
            objectFit: 'contain',
            display: 'block',
          }}
        />
      </Modal>
    </>
  );
}
