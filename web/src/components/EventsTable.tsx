import { Link } from "react-router-dom";
import {
  formatEventPortColumn,
  formatEventSourceLabel,
  formatEventSummary,
  formatEventTypeLabel,
} from "../eventFormat";
import { usePersistedColumnWidths } from "../hooks/usePersistedColumnWidths";
import type { EventRow } from "../types";
import type { DeviceBackRef } from "../navigation";

type DeviceLinkState = { deviceBack: DeviceBackRef };

type Props = {
  rows: EventRow[];
  deviceLabel: (deviceId: number) => string;
  deviceLinkState: DeviceLinkState;
  /** ключ localStorage для ширины колонок */
  widthStorageKey: string;
};

const DEFAULT_COL_WIDTHS = [150, 140, 220, 120, 100, 88, 400];

export default function EventsTable({ rows, deviceLabel, deviceLinkState, widthStorageKey }: Props) {
  const { colgroup, ResizeHandle } = usePersistedColumnWidths(widthStorageKey, DEFAULT_COL_WIDTHS);

  return (
    <table style={{ tableLayout: "fixed", width: "100%" }}>
      {colgroup}
      <thead>
        <tr>
          <th style={{ position: "relative", userSelect: "none" }}>
            Время
            <ResizeHandle colIndex={0} />
          </th>
          <th style={{ position: "relative", userSelect: "none" }}>
            Узел
            <ResizeHandle colIndex={1} />
          </th>
          <th style={{ position: "relative", userSelect: "none" }}>
            Порт
            <ResizeHandle colIndex={2} />
          </th>
          <th style={{ position: "relative", userSelect: "none" }}>
            Тип
            <ResizeHandle colIndex={3} />
          </th>
          <th style={{ position: "relative", userSelect: "none" }}>
            Серьёзность
            <ResizeHandle colIndex={4} />
          </th>
          <th style={{ position: "relative", userSelect: "none" }}>
            Источник
            <ResizeHandle colIndex={5} />
          </th>
          <th style={{ position: "relative", userSelect: "none" }}>
            Сводка
            <ResizeHandle colIndex={6} />
          </th>
        </tr>
      </thead>
      <tbody>
        {rows.map((ev) => (
          <tr key={ev.id}>
            <td style={{ whiteSpace: "nowrap" }}>{new Date(ev.created_at).toLocaleString()}</td>
            <td>
              <Link to={`/devices/${ev.device_id}`} state={deviceLinkState}>
                {deviceLabel(ev.device_id)}
              </Link>
            </td>
            <td
              style={{
                fontSize: "0.9rem",
                verticalAlign: "top",
                wordBreak: "break-word",
                overflowWrap: "anywhere",
              }}
            >
              {formatEventPortColumn(ev)}
            </td>
            <td>{formatEventTypeLabel(ev.event_type)}</td>
            <td>{ev.severity}</td>
            <td style={{ fontSize: "0.9rem", whiteSpace: "nowrap" }}>{formatEventSourceLabel(ev.payload)}</td>
            <td style={{ fontSize: "0.9rem", verticalAlign: "top", overflow: "hidden", textOverflow: "ellipsis" }}>
              {formatEventSummary(ev)}
              {typeof ev.payload?.mac === "string" && ev.payload.mac.trim() !== "" ? (
                <>
                  {" · "}
                  <Link to={`/investigate/mac?mac=${encodeURIComponent(String(ev.payload.mac).trim())}`}>
                    расследовать
                  </Link>
                </>
              ) : null}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
