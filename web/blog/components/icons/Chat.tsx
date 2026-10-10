import Svg, { type IconProps } from "./Svg";

/** 聊天气泡（AI 问答悬浮入口） */
export default function IconChat({ className }: IconProps) {
  return (
    <Svg className={className}>
      <path d="M8 10h8M8 14h5M21 12a9 9 0 11-4.2-7.6L21 3l-1.2 4.2A8.96 8.96 0 0121 12z" />
    </Svg>
  );
}
