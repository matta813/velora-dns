import { createContext, useContext } from "react";
import type { AuthUser } from "./api";

export const AuthUserContext = createContext<AuthUser | null>(null);

export function useAuthUser() {
  return useContext(AuthUserContext);
}
