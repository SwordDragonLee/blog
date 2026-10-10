import Svg, { type IconProps } from "./Svg";

/** 分享（三节点连线） */
export default function IconShare({ className }: IconProps) {
  return (
    <Svg className={className}>
      <circle cx="18" cy="5" r="3" />
      <circle cx="6" cy="12" r="3" />
      <circle cx="18" cy="19" r="3" />
      <line x1="8.59" y1="13.51" x2="15.42" y2="17.49" />
      <line x1="8.59" y1="10.49" x2="15.42" y2="6.51" />
    </Svg>
  );
}
