/* Shared API helpers for the login, appcatalog and egolifter apps.
 * Loaded via <script src="../shared/api.js"></script> BEFORE each page's own script.js. */
(function (global) {
  'use strict';

  // Backend base URL. The Go API listens on :8080 by default.
  const API_BASE = 'http://localhost:8080';

  const TOKEN_KEY = 'egolifter_token';
  const USER_KEY = 'egolifter_user';

  // --- Token + user storage (localStorage) ---
  function getToken() {
    return localStorage.getItem(TOKEN_KEY) || '';
  }
  function setToken(token) {
    if (token) localStorage.setItem(TOKEN_KEY, token);
  }
  function clearToken() {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
  }
  function setUser(user) {
    if (user) localStorage.setItem(USER_KEY, JSON.stringify(user));
  }
  function getUser() {
    try {
      return JSON.parse(localStorage.getItem(USER_KEY) || 'null');
    } catch (e) {
      return null;
    }
  }

  // --- Auth guard: bounce to the login page when no token is present. ---
  function requireAuth() {
    if (!getToken()) {
      window.location.replace('../login/');
      return false;
    }
    return true;
  }

  /* apiFetch performs a JSON request against the backend.
   * opts: { method, body }  (body is an object; it gets JSON-encoded)
   * Returns the parsed JSON response, or throws Error(message) on failure.
   * A 401 clears the token and redirects to login. */
  async function apiFetch(path, opts) {
    opts = opts || {};
    const headers = { 'Content-Type': 'application/json' };
    const token = getToken();
    if (token) headers['Authorization'] = 'Bearer ' + token;

    let res;
    try {
      res = await fetch(API_BASE + path, {
        method: opts.method || 'GET',
        headers: headers,
        credentials: 'include',
        body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      });
    } catch (e) {
      throw new Error('Cannot reach the server at ' + API_BASE + '. Is the backend running?');
    }

    if (res.status === 401) {
      clearToken();
      window.location.replace('../login/');
      throw new Error('Session expired. Please log in again.');
    }

    // Some endpoints (204) have no body.
    let data = null;
    const text = await res.text();
    if (text) {
      try {
        data = JSON.parse(text);
      } catch (e) {
        data = text;
      }
    }

    if (!res.ok) {
      const msg = data && data.message ? data.message : 'Request failed (' + res.status + ')';
      throw new Error(msg);
    }
    return data;
  }

  // --- Logout: best-effort server logout, then clear + redirect. ---
  async function logout() {
    try {
      await apiFetch('/auth/logout', { method: 'POST' });
    } catch (e) {
      /* ignore network/logout errors — we clear locally regardless */
    }
    clearToken();
    window.location.replace('../login/');
  }

  global.API = {
    API_BASE: API_BASE,
    getToken: getToken,
    setToken: setToken,
    clearToken: clearToken,
    setUser: setUser,
    getUser: getUser,
    requireAuth: requireAuth,
    apiFetch: apiFetch,
    logout: logout,
  };
})(window);
