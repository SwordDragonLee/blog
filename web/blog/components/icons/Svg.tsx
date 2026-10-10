/**
 * 线性图标统一外壳：24 viewBox、currentColor 描边、圆角端点、aria-hidden。
 * 仅供 icons/ 目录内各图标复用，不直接对外导出。
 */
import type { ReactNode, SVGProps } from "react";

export interface IconProps {
  /** 尺寸等样式由调用方传（h-4 w-4 等），缺省 h-4 w-4 */
  className?: string;
}

export default function Svg({
  className = "h-4 w-4",
  children,
  ...rest
}: SVGProps<SVGSVGElement> & { children: ReactNode }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className={className}
      {...rest}
    >
      {children}
    </svg>
  );
}
