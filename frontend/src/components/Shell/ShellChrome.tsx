import { createContext, useContext } from "react";

// The shell publishes only what a view header needs: whether the panel is
// currently hidden, and the two commands that reveal it. Components rendered
// outside the shell (tests, isolated stories) get inert defaults.
export interface ShellChrome {
  panelCollapsed: boolean;
  panelHidden: boolean;
  openPanel: () => void;
  togglePanel: () => void;
}

const inertChrome: ShellChrome = {
  panelCollapsed: false,
  panelHidden: false,
  openPanel: () => {},
  togglePanel: () => {},
};

const ShellChromeContext = createContext<ShellChrome>(inertChrome);

export const ShellChromeProvider = ShellChromeContext.Provider;

export function useShellChrome(): ShellChrome {
  return useContext(ShellChromeContext);
}
