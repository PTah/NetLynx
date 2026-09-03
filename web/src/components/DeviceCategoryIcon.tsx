import type { CSSProperties } from "react";
import { deviceIconUrl } from "../deviceIcons";

type Props = {
  category?: string | null;
  /** Высота в px; ширина по пропорции ~16:9. */
  height?: number;
  title?: string;
  style?: CSSProperties;
  className?: string;
};

/** Иконка типа устройства (стандартный пак; white layer только в email). */
export function DeviceCategoryIcon({
  category,
  height = 22,
  title,
  style,
  className,
}: Props) {
  const w = Math.round(height * (160 / 89));
  return (
    <img
      className={className}
      src={deviceIconUrl(category)}
      alt=""
      title={title}
      width={w}
      height={height}
      draggable={false}
      style={{
        display: "inline-block",
        verticalAlign: "middle",
        objectFit: "contain",
        flexShrink: 0,
        ...style,
      }}
    />
  );
}
