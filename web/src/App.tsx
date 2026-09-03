import { Navigate, Route, Routes } from "react-router-dom";
import ErrorBoundary from "./components/ErrorBoundary";
import RequireAuth from "./components/RequireAuth";
import ShellLayout from "./layout/ShellLayout";
import Dashboard from "./pages/Dashboard";
import DeviceDetail from "./pages/DeviceDetail";
import Devices from "./pages/Devices";
import Events from "./pages/Events";
import InvestigateMAC from "./pages/InvestigateMAC";
import InvestigateLoops from "./pages/InvestigateLoops";
import Login from "./pages/Login";
import Postmortem from "./pages/Postmortem";
import Settings from "./pages/Settings";
import Topology from "./pages/Topology";
import Discovered from "./pages/Discovered";

export default function App() {
  return (
    <ErrorBoundary>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route element={<RequireAuth />}>
          <Route element={<ShellLayout />}>
            <Route
              index
              element={
                <ErrorBoundary title="Ошибка дашборда">
                  <Dashboard />
                </ErrorBoundary>
              }
            />
            <Route
              path="devices"
              element={
                <ErrorBoundary title="Ошибка списка узлов">
                  <Devices />
                </ErrorBoundary>
              }
            />
            <Route
              path="devices/:id"
              element={
                <ErrorBoundary title="Ошибка карточки узла">
                  <DeviceDetail />
                </ErrorBoundary>
              }
            />
            <Route
              path="topology"
              element={
                <ErrorBoundary title="Ошибка топологии">
                  <Topology />
                </ErrorBoundary>
              }
            />
            <Route
              path="discovered"
              element={
                <ErrorBoundary title="Ошибка «Обнаружено»">
                  <Discovered />
                </ErrorBoundary>
              }
            />
            <Route
              path="events"
              element={
                <ErrorBoundary title="Ошибка событий">
                  <Events />
                </ErrorBoundary>
              }
            />
            <Route
              path="investigate/mac"
              element={
                <ErrorBoundary title="Ошибка расследования MAC">
                  <InvestigateMAC />
                </ErrorBoundary>
              }
            />
            <Route
              path="investigate/loops"
              element={
                <ErrorBoundary title="Ошибка поиска петель">
                  <InvestigateLoops />
                </ErrorBoundary>
              }
            />
            <Route
              path="postmortem"
              element={
                <ErrorBoundary title="Ошибка Postmortem">
                  <Postmortem />
                </ErrorBoundary>
              }
            />
            <Route
              path="settings"
              element={
                <ErrorBoundary title="Ошибка настроек">
                  <Settings />
                </ErrorBoundary>
              }
            />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Route>
      </Routes>
    </ErrorBoundary>
  );
}
