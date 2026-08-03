import { useEffect, useState } from "react";

export function useElapsedTime(start?: string, active = false): string {
  const [now, setNow] = useState(Date.now());

  useEffect(() => {
    setNow(Date.now());
    if (!active) return undefined;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [active, start]);

  return formatElapsedTime(start, active, now);
}

export function formatElapsedTime(start: string | undefined, active: boolean, now: number): string {
  if (!start || !active) return "--:--:--";
  const startedAt = new Date(start).getTime();
  if (Number.isNaN(startedAt)) return "--:--:--";
  const seconds = Math.max(0, Math.floor((now - startedAt) / 1000));
  const hours = String(Math.floor(seconds / 3600)).padStart(2, "0");
  const minutes = String(Math.floor(seconds / 60) % 60).padStart(2, "0");
  return `${hours}:${minutes}:${String(seconds % 60).padStart(2, "0")}`;
}
