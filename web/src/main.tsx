import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { AuthGate } from "./AuthGate";
import { LanguageProvider } from "./LanguageProvider";
import "./styles.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <LanguageProvider>
        <AuthGate><App /></AuthGate>
      </LanguageProvider>
    </BrowserRouter>
  </StrictMode>,
);