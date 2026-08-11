import { Area, AreaChart, CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { TrafficPoint } from "./useTrafficHistory";

export function TrafficSparkline({ points }: { points: TrafficPoint[] }) {
  const data = chartPoints(points);
  return (
    <div className="sparkline" aria-label="实时上传流量趋势">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 8, right: 0, bottom: 0, left: 0 }}>
          <Area type="monotone" dataKey="upload" stroke="#12ad67" strokeWidth={2} fill="#e3f7ec" fillOpacity={0.9} isAnimationActive animationDuration={650} animationEasing="ease-out" />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

export function TrafficTrend({ points }: { points: TrafficPoint[] }) {
  const data = chartPoints(points);
  return (
    <div className="traffic-chart" aria-label="实时上传和下载流量趋势">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 12, right: 12, bottom: 0, left: -18 }}>
          <CartesianGrid stroke="#edf1ef" vertical={false} />
          <XAxis dataKey="time" tick={{ fill: "#7b8798", fontSize: 11 }} axisLine={false} tickLine={false} minTickGap={28} />
          <YAxis tick={{ fill: "#7b8798", fontSize: 11 }} axisLine={false} tickLine={false} />
          <Tooltip contentStyle={{ borderRadius: 8, borderColor: "#dfe7e3", fontSize: 12 }} />
          <Line type="monotone" dataKey="upload" name="上传 KB/s" stroke="#12ad67" strokeWidth={2} dot={false} isAnimationActive animationDuration={650} animationEasing="ease-out" />
          <Line type="monotone" dataKey="download" name="下载 KB/s" stroke="#2f7df6" strokeWidth={2} dot={false} isAnimationActive animationDuration={650} animationEasing="ease-out" />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

function seedPoints(): TrafficPoint[] {
  return [120, 180, 145, 220, 170, 260, 210, 280, 235, 310, 250, 295].map((upload, index) => ({
    time: `-${11 - index}s`, upload, download: Math.round(upload * 0.42 + (index % 3) * 18),
  }));
}

function chartPoints(points: TrafficPoint[]) {
  if (import.meta.env.DEV) return [...seedPoints(), ...points];
  return points.length > 1 ? points : [
    { time: "-1s", upload: 0, download: 0 },
    { time: "now", upload: 0, download: 0 },
  ];
}
