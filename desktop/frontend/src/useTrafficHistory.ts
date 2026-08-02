import { useEffect, useRef, useState } from "react";
import type { RuntimeStatus } from "../bindings/github.com/nat-link/nat-link/desktop";

export interface TrafficPoint {
  time: string;
  upload: number;
  download: number;
}

export function useTrafficHistory(status?: RuntimeStatus) {
  const [points, setPoints] = useState<TrafficPoint[]>([]);
  const [rates, setRates] = useState({ upload: 0, download: 0 });
  const previous = useRef<{ upload: number; download: number; time: number }>();

  useEffect(() => {
    if (!status) return;
    const now = Date.now();
    const last = previous.current;
    previous.current = { upload: status.uploadBytes, download: status.downloadBytes, time: now };
    if (!last || now - last.time < 1000) return;
    const seconds = Math.max(0.25, (now - last.time) / 1000);
    const upload = status.connected ? Math.max(0, (status.uploadBytes - last.upload) / seconds / 1024) : 0;
    const download = status.connected ? Math.max(0, (status.downloadBytes - last.download) / seconds / 1024) : 0;
    setRates({ upload, download });
    setPoints((current) => [...current, {
      time: new Date(now).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
      upload: Math.round(upload), download: Math.round(download),
    }].slice(-32));
  }, [status?.uploadBytes, status?.downloadBytes, status?.connected]);

  return { points, rates };
}
