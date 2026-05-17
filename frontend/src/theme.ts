import { get, writable } from 'svelte/store';

export type ThemeMode = 'system' | 'light' | 'dark';

const THEME_STORAGE_KEY = 'mx-api-go.admin.theme';

export const supportedThemeModes = Object.freeze(['system', 'light', 'dark'] as const);
export const themePreference = writable<ThemeMode>('system');
export const resolvedTheme = writable<'light' | 'dark'>('light');

let cleanupMediaQuery = () => {};

function normalizeThemePreference(value: string | null): ThemeMode {
  return supportedThemeModes.includes(value as ThemeMode) ? (value as ThemeMode) : 'system';
}

function resolveTheme(preference: ThemeMode): 'light' | 'dark' {
  if (preference === 'system') {
    return typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  return preference;
}

function applyResolvedTheme(theme: 'light' | 'dark') {
  if (typeof document === 'undefined') return;
  document.documentElement.classList.toggle('dark', theme === 'dark');
  document.documentElement.style.colorScheme = theme;
  resolvedTheme.set(theme);
}

function syncTheme() {
  applyResolvedTheme(resolveTheme(get(themePreference)));
}

function attachMediaQueryListener() {
  if (typeof window === 'undefined') return () => {};
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
  const handleChange = () => {
    if (get(themePreference) === 'system') syncTheme();
  };

  if (typeof mediaQuery.addEventListener === 'function') {
    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }

  mediaQuery.addListener(handleChange);
  return () => mediaQuery.removeListener(handleChange);
}

export function setupTheme() {
  const storedTheme = typeof window === 'undefined' ? 'system' : window.localStorage.getItem(THEME_STORAGE_KEY);
  themePreference.set(normalizeThemePreference(storedTheme));
  cleanupMediaQuery();
  cleanupMediaQuery = attachMediaQueryListener();
  syncTheme();
  return cleanupTheme;
}

export function cleanupTheme() {
  cleanupMediaQuery();
  cleanupMediaQuery = () => {};
}

export function setThemeMode(nextTheme: ThemeMode) {
  const normalized = normalizeThemePreference(nextTheme);
  themePreference.set(normalized);
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(THEME_STORAGE_KEY, normalized);
  }
  syncTheme();
}
