import { useCallback, useEffect, useState } from "react";
import { ApiError } from "../api/client";

export interface AsyncState<T> {
  data?: T;
  loading: boolean;
  error?: string;
}

export function useAsync<T>(loader: () => Promise<T>, deps: unknown[] = []) {
  const [state, setState] = useState<AsyncState<T>>({ loading: true });

  const load = useCallback(async () => {
    setState((current) => ({ ...current, loading: true, error: undefined }));
    try {
      const data = await loader();
      setState({ data, loading: false });
    } catch (error) {
      setState({ loading: false, error: toMessage(error) });
    }
  }, deps);

  useEffect(() => {
    void load();
  }, [load]);

  return { ...state, reload: load };
}

export function toMessage(error: unknown) {
  if (error instanceof ApiError && error.status === 404) return "接口尚未提供或资源不存在";
  if (error instanceof Error) return error.message;
  return "请求失败";
}
