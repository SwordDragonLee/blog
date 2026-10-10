import Svg, { type IconProps } from "./Svg";

/** 停止（实心方块，回答生成中） */
export default function IconStop({ className }: IconProps) {
  return (
    <Svg className={className}>
      <rect x="7" y="7" width="10" height="10" rx="1.5" fill="currentColor" stroke="none" />
    </Svg>
  );
}
