// auth.tsx — toy bearer-token auth (api/middleware.go §2.1: the token subject IS the
// principal). The token lives in sessionStorage (not localStorage) so it clears on tab
// close and never outlives the session — a money toy should not leave a bearer token in
// persistent storage. Never logged, never put in a URL.

import { createContext, useCallback, useContext, useMemo, useState } from "react";
import type { ReactNode } from "react";

const STORAGE_KEY = "ledger.token";

interface AuthState {
  token: string | null;
  principal: string | null; // toy: the principal is the token subject === the token
  signIn: (token: string) => void;
  signOut: () => void;
  getToken: () => string | null;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => {
    try {
      return sessionStorage.getItem(STORAGE_KEY);
    } catch {
      return null;
    }
  });

  const signIn = useCallback((t: string) => {
    const trimmed = t.trim();
    try {
      sessionStorage.setItem(STORAGE_KEY, trimmed);
    } catch {
      /* sessionStorage can be unavailable; hold it in memory regardless */
    }
    setToken(trimmed);
  }, []);

  const signOut = useCallback(() => {
    try {
      sessionStorage.removeItem(STORAGE_KEY);
    } catch {
      /* ignore */
    }
    setToken(null);
  }, []);

  // getToken reads live state via a ref-free closure over the latest token. useMemo
  // rebinds it whenever token changes so the client always sends the current token.
  const value = useMemo<AuthState>(
    () => ({
      token,
      principal: token, // toy: subject-is-token (api/handler.go subjectIsToken)
      signIn,
      signOut,
      getToken: () => token,
    }),
    [token, signIn, signOut],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (ctx === null) throw new Error("useAuth must be used within an AuthProvider");
  return ctx;
}
