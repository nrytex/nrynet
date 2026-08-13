import { useEffect, useRef, useState } from "react";
import type { RuntimeStatus } from "../bindings/github.com/nrytex/nrynet/desktop";

const rateWindowMs = 1000;
const maxHistoryPoints = 300;

export interface TrafficPoint {
  time: string;
  upload: number;
  download: number;
}

interface TrafficSample {
  time: number;
  upload: number;
  download: number;
}

export function useTrafficHistory(status?: RuntimeStatus) {
  const [points, setPoints] = useState<TrafficPoint[]>([]);
  const [rates, setRates] = useState({ upload: 0, download: 0 });
  const samples = useRef<TrafficSample[]>([]);
  const previousConnectionState = useRef<boolean>();

  useEffect(() => {
    if (!status) return;
    const now = Date.now();
    const sample: TrafficSample = { time: now, upload: status.uploadBytes, download: status.downloadBytes };
    const stateChanged = previousConnectionState.current !== undefined && previousConnectionState.current !== status.connected;
    previousConnectionState.current = status.connected;
    if (stateChanged || samples.current.length === 0) samples.current = [sample];
    else samples.current = [...samples.current.filter((item) => now - item.time <= rateWindowMs * 10), sample];

    const baseline = samples.current.find((item) => now - item.time >= rateWindowMs) ?? samples.current[0];
    const seconds = Math.max(0.25, (now - baseline.time) / 1000);
    const upload = status.connected ? Math.max(0, (status.uploadBytes - baseline.upload) / seconds / 1024) : 0;
    const download = status.connected ? Math.max(0, (status.downloadBytes - baseline.download) / seconds / 1024) : 0;
    setRates({ upload, download });
    setPoints((current) => [...current, {
      time: new Date(now).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
      upload: Number(upload.toFixed(1)), download: Number(download.toFixed(1)),
    }].slice(-maxHistoryPoints));
  }, [status]);

  return { points, rates };
}
