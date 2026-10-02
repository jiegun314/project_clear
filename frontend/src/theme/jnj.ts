import type { ThemeConfig } from 'antd';

/**
 * Johnson & Johnson palette.
 *
 * The brief calls for the company's default red with neutral greys and white.
 * J&J's brand red is #D71920; everything else here is a neutral so the data
 * grid stays the loudest thing on screen.
 */
export const JNJ = {
  red: '#D71920',
  redHover: '#B0151A',
  redActive: '#8E1014',
  redSoft: '#FCEDEE',
  redBorder: '#F3C2C5',

  ink: '#1F1F1F',
  text: '#525252',
  textMuted: '#8C8C8C',
  border: '#E0E0E0',
  divider: '#F0F0F0',

  bg: '#F4F4F5',
  surface: '#FFFFFF',
  surfaceAlt: '#FAFAFA',
  fill: '#F2F2F2',

  success: '#1F7A3D',
  successSoft: '#EDF7F0',
  warning: '#B26A00',
  warningSoft: '#FFF6E8',
  danger: '#D71920',
  dangerSoft: '#FCEDEE',
} as const;

/** Ant Design token set derived from the palette above. */
export const theme: ThemeConfig = {
  token: {
    colorPrimary: JNJ.red,
    colorInfo: JNJ.red,
    colorSuccess: JNJ.success,
    colorWarning: JNJ.warning,
    colorError: JNJ.danger,
    colorTextBase: JNJ.ink,
    colorBorder: JNJ.border,
    colorBorderSecondary: JNJ.divider,
    colorBgLayout: JNJ.bg,
    colorBgContainer: JNJ.surface,
    colorFillAlter: JNJ.surfaceAlt,
    borderRadius: 4,
    fontSize: 13,
    controlHeight: 32,
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Helvetica Neue", Arial, sans-serif',
    wireframe: false,
  },
  components: {
    Button: {
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      fontWeight: 500,
    },
    Table: {
      headerBg: JNJ.surfaceAlt,
      headerColor: JNJ.text,
      headerSplitColor: JNJ.border,
      borderColor: JNJ.divider,
      rowHoverBg: JNJ.redSoft,
      cellPaddingBlock: 6,
      cellPaddingInline: 10,
      headerBorderRadius: 0,
    },
    Tabs: {
      itemColor: JNJ.text,
      itemSelectedColor: JNJ.red,
      itemHoverColor: JNJ.redHover,
      inkBarColor: JNJ.red,
    },
    Modal: {
      titleFontSize: 16,
      headerBg: JNJ.surface,
    },
    Card: {
      headerFontSize: 14,
    },
    Segmented: {
      itemSelectedBg: JNJ.red,
      itemSelectedColor: '#FFFFFF',
    },
    Tag: {
      defaultBg: JNJ.fill,
    },
    Input: {
      activeBorderColor: JNJ.red,
      hoverBorderColor: JNJ.redHover,
    },
    Select: {
      optionSelectedBg: JNJ.redSoft,
    },
  },
};

/** The red used for module titles: red plate, white lettering. */
export const moduleTitleStyle: React.CSSProperties = {
  background: JNJ.red,
  color: '#FFFFFF',
  fontWeight: 600,
  letterSpacing: 0.5,
};
