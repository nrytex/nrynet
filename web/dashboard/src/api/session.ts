const SESSION_KEY = "nat-link.session";

export interface Session {
  token: string;
  tokenType: string;
}

export function getSession(): Session | null {
  const raw = window.localStorage.getItem(SESSION_KEY);
  if (!raw) return null;
  try {
    const session = JSON.parse(raw) as Session;
    return session.token ? session : null;
  } catch {
    clearSession();
    return null;
  }
}

export function saveSession(token: string, tokenType = "Bearer") {
  window.localStorage.setItem(SESSION_KEY, JSON.stringify({ token, tokenType }));
}

export function clearSession() {
  window.localStorage.removeItem(SESSION_KEY);
}

export function authHeader(): Record<string, string> {
  const session = getSession();
  return session ? { Authorization: `${session.tokenType} ${session.token}` } : {};
}
