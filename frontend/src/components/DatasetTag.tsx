import { Tag } from 'antd';
import { JNJ } from '../theme/jnj';

/** How the three datasets are named in the list and in the archive window. */
export const DATASET_LABEL: Record<string, string> = { merged: '整合', global: 'Global', local: 'Local' };

/**
 * Which dataset a row or a window belongs to, as a tag: with three rows per week
 * it is the first thing the eye needs in order to tell them apart. 整合 is the
 * deliverable and carries the brand red; Global stays neutral; Local is the soft
 * red outline.
 */
export function DatasetTag({ dataset }: { dataset: string }) {
  const style =
    dataset === 'merged'
      ? { background: JNJ.red, borderColor: JNJ.red, color: '#fff', fontWeight: 600 }
      : dataset === 'local'
        ? { background: JNJ.redSoft, borderColor: JNJ.redBorder, color: JNJ.red }
        : { background: JNJ.surface, borderColor: JNJ.border, color: JNJ.text };
  return (
    <Tag
      style={{
        // A fixed box so 整合 / Global / Local line up as one column of labels;
        // the width is the natural width of the longest label, Global.
        margin: 0,
        padding: 0,
        width: 45,
        height: 20,
        boxSizing: 'border-box',
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        flex: '0 0 45px',
        fontSize: 11,
        lineHeight: '18px',
        borderRadius: 3,
        whiteSpace: 'nowrap',
        verticalAlign: 'middle',
        ...style,
      }}
    >
      {DATASET_LABEL[dataset] ?? dataset}
    </Tag>
  );
}
