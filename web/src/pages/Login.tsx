import { FormEvent, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { login } from "../auth";

type NavState = { from?: { pathname?: string } };

export default function Login() {
  const nav = useNavigate();
  const loc = useLocation();
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    login(username.trim(), password)
      .then(() => {
        const st = (loc.state as NavState | null)?.from?.pathname;
        nav(st || "/", { replace: true });
      })
      .catch((e: Error) => setErr(e.message))
      .finally(() => setBusy(false));
  };

  return (
    <div className="login-page">
      <form className="login-form" onSubmit={onSubmit}>
        <img className="login-form-logo" src="/logo.png" alt="NetLynx" width={72} height={72} />
        <h1>Вход в NetLynx</h1>
        {err && <p className="login-form-error">{err}</p>}
        <label>
          Логин
          <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
        </label>
        <label>
          Пароль
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />
        </label>
        <button type="submit" disabled={busy}>
          {busy ? "Входим..." : "Войти"}
        </button>
      </form>
    </div>
  );
}
