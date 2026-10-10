import Svg, { type IconProps } from "./Svg";

/** 垃圾桶（清空对话） */
export default function IconTrash({ className }: IconProps) {
  return (
    <Svg className={className}>
      <path d="M3 6h18M8 6V4a1 1 0 011-1h6a1 1 0 011 1v2m2 0v14a2 2 0 01-2 2H8a2 2 0 01-2-2V6h12z" />
    </Svg>
  );
}
