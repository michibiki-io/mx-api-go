/// <reference types="vite/client" />

declare global {
  interface Window {
    MX_API_ADMIN?: {
      apiBasePath: string;
      dashboardBasePath: string;
      dashboardToken?: string;
    };
  }
}

export {};
