import { useEffect, useRef, useState } from "react";
import type { TrafficPoint } from "./useTrafficHistory";

export function TrafficSparkline({ points }: { points: TrafficPoint[] }) {
  return <TrafficCanvas points={points} className="sparkline" ariaLabel="实时上传流量趋势" />;
}

export function TrafficTrend({ points }: { points: TrafficPoint[] }) {
  return <TrafficCanvas points={points} className="traffic-chart" detailed ariaLabel="实时上传和下载流量趋势" />;
}

function TrafficCanvas({ points, className, detailed, ariaLabel }: {
  points: TrafficPoint[];
  className: string;
  detailed?: boolean;
  ariaLabel: string;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const previousLast = useRef({ upload: 0, download: 0 });
  const frameRef = useRef<number>();
  const [size, setSize] = useState({ width: 0, height: 0 });

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const updateSize = () => {
      const rect = canvas.getBoundingClientRect();
      setSize({ width: Math.round(rect.width), height: Math.round(rect.height) });
    };
    updateSize();
    const observer = new ResizeObserver(updateSize);
    observer.observe(canvas);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || size.width <= 0 || size.height <= 0) return;
    const devicePixelRatio = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(size.width * devicePixelRatio);
    canvas.height = Math.round(size.height * devicePixelRatio);
    const context = canvas.getContext("2d");
    if (!context) return;
    const last = points.length > 0 ? points[points.length - 1] : { upload: 0, download: 0 };
    const start = previousLast.current;
    previousLast.current = { upload: last.upload, download: last.download };
    if (frameRef.current !== undefined) cancelAnimationFrame(frameRef.current);
    const drawFrame = () => {
      const upload = start.upload + (last.upload - start.upload);
      const download = start.download + (last.download - start.download);
      drawChart(context, points, size.width, size.height, devicePixelRatio, detailed, upload, download);
    };
    frameRef.current = requestAnimationFrame(drawFrame);
    return () => {
      if (frameRef.current !== undefined) cancelAnimationFrame(frameRef.current);
    };
  }, [detailed, points, size]);

  return <canvas ref={canvasRef} className={className} aria-label={ariaLabel} role="img" />;
}

function drawChart(
  context: CanvasRenderingContext2D,
  points: TrafficPoint[],
  width: number,
  height: number,
  ratio: number,
  detailed: boolean | undefined,
  lastUpload: number,
  lastDownload: number,
) {
  context.save();
  context.scale(ratio, ratio);
  context.clearRect(0, 0, width, height);
  const inset = detailed ? { top: 12, right: 12, bottom: 8, left: 30 } : { top: 8, right: 2, bottom: 4, left: 2 };
  const chartWidth = Math.max(1, width - inset.left - inset.right);
  const chartHeight = Math.max(1, height - inset.top - inset.bottom);
  const values = points.length > 80
    ? points.filter((_, index) => index % 2 === 0).flatMap((point) => [point.upload, point.download])
    : points.flatMap((point) => [point.upload, point.download]);
  const maximum = Math.max(1, ...values, lastUpload, lastDownload) * 1.15;
  const x = (index: number) => inset.left + (points.length <= 1 ? chartWidth : index / (points.length - 1) * chartWidth);
  const y = (value: number) => inset.top + chartHeight - value / maximum * chartHeight;

  if (detailed) drawGrid(context, inset.left, inset.top, chartWidth, chartHeight);
  drawLine(context, points, x, y, "#12ad67", "upload", lastUpload);
  if (detailed) drawLine(context, points, x, y, "#2f7df6", "download", lastDownload);
  context.restore();
}

function drawGrid(context: CanvasRenderingContext2D, left: number, top: number, width: number, height: number) {
  context.strokeStyle = "#edf1ef";
  context.lineWidth = 1;
  for (let index = 1; index < 4; index += 1) {
    const y = top + height * index / 4;
    context.beginPath();
    context.moveTo(left, y);
    context.lineTo(left + width, y);
    context.stroke();
  }
}

function drawLine(
  context: CanvasRenderingContext2D,
  points: TrafficPoint[],
  x: (index: number) => number,
  y: (value: number) => number,
  color: string,
  key: "upload" | "download",
  lastValue: number,
) {
  if (points.length === 0) return;
  context.strokeStyle = color;
  context.lineWidth = 2;
  context.lineJoin = "round";
  context.lineCap = "round";
  context.beginPath();
  points.forEach((point, index) => {
    const value = index === points.length - 1 ? lastValue : point[key];
    if (index === 0) context.moveTo(x(index), y(value));
    else context.lineTo(x(index), y(value));
  });
  context.stroke();
}
