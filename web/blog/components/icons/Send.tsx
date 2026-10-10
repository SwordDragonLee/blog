import Svg, { type IconProps } from "./Svg";

/** 发送（右箭头） */
export default function IconSend({ className }: IconProps) {
  return (
    <Svg className={className}>
      <path d="M5 12h14M13 6l6 6-6 6" />
    </Svg>
  );
}
