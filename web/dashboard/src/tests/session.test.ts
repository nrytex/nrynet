import { beforeEach, describe, expect, it } from "vitest";
import { authHeader, clearSession, getSession, saveSession } from "../api/session";

describe("session", () => {
  beforeEach(() => localStorage.clear());

  it("persists bearer session", () => {
    saveSession("abc");
    expect(getSession()).toEqual({ token: "abc", tokenType: "Bearer" });
    expect(authHeader()).toEqual({ Authorization: "Bearer abc" });
  });

  it("clears invalid stored session", () => {
    localStorage.setItem("nat-link.session", "{bad");
    expect(getSession()).toBeNull();
    expect(localStorage.getItem("nat-link.session")).toBeNull();
  });

  it("removes session", () => {
    saveSession("abc");
    clearSession();
    expect(getSession()).toBeNull();
  });
});
