import { BrowserRouter, Route, Routes } from "react-router-dom";
import { Layout } from "./components/Layout";
import { ToastProvider } from "./components/toast";
import { QuickTest } from "./pages/QuickTest";
import { Monitoring } from "./pages/Monitoring";
import { ServerDetail } from "./pages/ServerDetail";
import { Settings } from "./pages/Settings";

export default function App() {
  return (
    <ToastProvider>
      <BrowserRouter>
        <Routes>
          <Route element={<Layout />}>
            <Route index element={<QuickTest />} />
            <Route path="monitoring" element={<Monitoring />} />
            <Route path="monitoring/:id" element={<ServerDetail />} />
            <Route path="settings" element={<Settings />} />
            <Route path="*" element={<QuickTest />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ToastProvider>
  );
}
