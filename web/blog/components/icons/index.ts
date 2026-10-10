/**
 * 项目内联 SVG 图标统一出口：import { IconXxx } from "@/components/icons"。
 *
 * 项目不引图标库（图标总量少、均为 24 viewBox 线性风格）；图标一文件一组件放
 * 本目录，经此 index 统一 re-export。约定：
 * - 新图标：本目录建文件（默认导出 IconXxx，用 Svg 外壳），在此加一行 re-export；
 * - 组件里不要再写内联 <svg>；
 * - 颜色一律 currentColor（Tailwind text-* 控制），实心图标在元素级覆盖 fill；
 * - 不带 "use client"：服务端/客户端组件均可引用。
 */

export { default as IconChat } from "./Chat";
export { default as IconClose } from "./Close";
export { default as IconSend } from "./Send";
export { default as IconStop } from "./Stop";
export { default as IconTrash } from "./Trash";
export { default as IconShare } from "./Share";
export { default as IconHeart } from "./Heart";
