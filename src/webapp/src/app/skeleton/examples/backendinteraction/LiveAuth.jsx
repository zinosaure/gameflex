import React, { useContext, useEffect, useState } from 'react';
import * as bs from 'react-bootstrap';
import Skeleton from "../../skeleton.js";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

// Demo-only: a real app would keep the token in memory + a refresh-token/httpOnly
// cookie flow instead of localStorage, but localStorage keeps this example short.
const TOKEN_STORAGE_KEY = "skeleton-demo-auth-token";

async function api(url, options = {}) {
    const res = await fetch(url, options);

    if (res.status === 204) {
        return { state: "success", item: null, ok: true };
    }

    const json = await res.json().catch(() => ({ state: "failed", exception: { detail: "Invalid server response" } }));

    return { ...json, ok: res.ok };
}

function authHeaders(token) {
    return token ? { Authorization: `Bearer ${token}` } : {};
}

// Shared username/password form for both the register and login cards below.
function CredentialsForm({ submitLabel, onSubmit, isPending }) {
    const [values, setValues] = useState({ username: "", password: "" });
    const rulesets = {
        username: (data) => (!data.username || data.username.length < 3) ? Ruleset.isInvalid("At least 3 characters") : null,
        password: (data) => (!data.password || data.password.length < 6) ? Ruleset.isInvalid("At least 6 characters") : null,
    };
    const formContext = { values, setValues, files: {}, setFiles: () => { }, rulesets };
    const isValid = Ruleset.hasFeedbacks(formContext) === null;

    return (
        <div>
            <Form>
                <Form.Group label="Username"><Form.Text name="username" context={formContext} props={{ autoComplete: "username" }} /></Form.Group>
                <Form.Group label="Password"><Form.Text name="password" context={formContext} props={{ type: "password", autoComplete: "current-password" }} /></Form.Group>
            </Form>
            <bs.Button
                className="mt-2"
                variant="primary"
                disabled={!isValid || isPending}
                onClick={() => onSubmit(values)}>
                {isPending ? <bs.Spinner size="sm" animation="border" /> : submitLabel}
            </bs.Button>
        </div>
    );
}

export default function LiveAuth() {
    const context = useContext(Skeleton.context);
    const [token, setToken] = useState(() => localStorage.getItem(TOKEN_STORAGE_KEY) || "");
    const [user, setUser] = useState(null);
    const [isPending, setPending] = useState(false);
    const [secretResult, setSecretResult] = useState(null);

    const fetchMe = async (currentToken) => {
        if (!currentToken) {
            setUser(null);
            return;
        }

        const result = await api("/api/auth/me", { headers: authHeaders(currentToken) });

        if (result.ok) {
            setUser(result.item);
        } else {
            // Token expired, or the users table got wiped by "Reset demo data" — drop it.
            setToken("");
            localStorage.removeItem(TOKEN_STORAGE_KEY);
            setUser(null);
        }
    };

    useEffect(() => { fetchMe(token); }, []);

    const handleAuthResult = (result, successMessage) => {
        if (result.ok) {
            setToken(result.item.access_token);
            localStorage.setItem(TOKEN_STORAGE_KEY, result.item.access_token);
            setUser(result.item.user);
            setSecretResult(null);
            context.toast.open(successMessage, { variant: "success" });
        } else {
            context.toast.open(result.exception?.detail || "Request failed", { variant: "error" });
        }
    };

    const register = async (values) => {
        setPending(true);

        const result = await api("/api/auth/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(values),
        });

        setPending(false);
        handleAuthResult(result, `Account "${values.username}" created — logged in`);
    };

    const login = async (values) => {
        setPending(true);

        const result = await api("/api/auth/login", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(values),
        });

        setPending(false);
        handleAuthResult(result, `Logged in as "${values.username}"`);
    };

    const logout = () => {
        setToken("");
        setUser(null);
        setSecretResult(null);
        localStorage.removeItem(TOKEN_STORAGE_KEY);
        context.toast.open("Logged out", { variant: "info" });
    };

    const callSecret = async (withToken) => {
        const result = await api("/api/auth/secret", { headers: withToken ? authHeaders(token) : {} });
        setSecretResult(result);
    };

    const confirmResetDatabase = () => {
        context.modal.open(
            "This resets the entire demo dataset — games, consoles, their links, and every registered account (they all share the same in-memory database) — back to the original seed data.",
            {
                variant: "confirm",
                title: "Reset demo database",
                confirmLabel: "Reset",
                cancelLabel: "Cancel",
                onConfirm: async () => {
                    const result = await api("/api/retrogaming/reset", { method: "POST" });

                    if (result.ok) {
                        context.toast.open("Demo database reset — you've been logged out", { variant: "success" });
                        logout();
                    } else {
                        context.toast.open("Reset failed", { variant: "error" });
                    }
                },
            }
        );
    };

    return (
        <div className="container">
            <div className="mb-4 d-flex justify-content-between align-items-start flex-wrap gap-2">
                <div>
                    <h1 className="display-5 fw-bold mb-3">Live Auth</h1>
                    <p className="lead text-muted mb-0">
                        A minimal JWT auth flow against the real backend (<code>/api/auth/register</code>, <code>/api/auth/login</code>, <code>/api/auth/me</code>) —
                        register or log in, then call a route protected with <code>Depends(get_current_user)</code> to see the token actually enforced.
                    </p>
                </div>
                <bs.Button variant="outline-danger" size="sm" onClick={confirmResetDatabase} className="text-nowrap">
                    <i className="bi bi-arrow-counterclockwise me-1"></i>Reset demo data
                </bs.Button>
            </div>

            {user ? (
                <bs.Alert variant="success" className="d-flex justify-content-between align-items-center">
                    <div>
                        Logged in as <strong>{user.username}</strong> (id {user.id}) — account created {new Date(user.created_at).toLocaleString()}.
                    </div>
                    <bs.Button size="sm" variant="outline-secondary" onClick={logout}>Log out</bs.Button>
                </bs.Alert>
            ) : (
                <bs.Row className="g-4 mb-4">
                    <bs.Col md={6}>
                        <bs.Card body>
                            <h2 className="h5">Register</h2>
                            <p className="text-muted small">Creates a new account (password hashed with bcrypt) and logs you in immediately.</p>
                            <CredentialsForm submitLabel="Create account" onSubmit={register} isPending={isPending} />
                        </bs.Card>
                    </bs.Col>
                    <bs.Col md={6}>
                        <bs.Card body>
                            <h2 className="h5">Log in</h2>
                            <p className="text-muted small">Use an account you already registered above.</p>
                            <CredentialsForm submitLabel="Log in" onSubmit={login} isPending={isPending} />
                        </bs.Card>
                    </bs.Col>
                </bs.Row>
            )}

            <bs.Card body className="mb-4">
                <h2 className="h5">Call a protected route</h2>
                <p className="text-muted small mb-3">
                    <code>GET /api/auth/secret</code> requires <code>Depends(get_current_user)</code> server-side — it 401s without a valid bearer token, regardless of whether you're logged in on this page.
                </p>
                <div className="d-flex gap-2 mb-3">
                    <bs.Button variant="primary" disabled={!token} onClick={() => callSecret(true)}>
                        <i className="bi bi-shield-lock me-1"></i>Call with token
                    </bs.Button>
                    <bs.Button variant="outline-secondary" onClick={() => callSecret(false)}>
                        Call without token
                    </bs.Button>
                </div>
                {secretResult ? (
                    <bs.Alert variant={secretResult.ok ? "success" : "danger"} className="mb-0">
                        <strong>{secretResult.ok ? "200 OK" : "401 Unauthorized"}</strong> —{" "}
                        {secretResult.ok ? secretResult.item.message : (secretResult.exception?.detail || "Request failed")}
                    </bs.Alert>
                ) : null}
            </bs.Card>

            <bs.Card body>
                <h2 className="h5">How it's wired</h2>
                <pre className="bg-body-tertiary border rounded p-3 small mb-0"><code>{`// Frontend: attach the token from localStorage to any protected call.
fetch("/api/auth/secret", {
    headers: { Authorization: \`Bearer \${token}\` },
});

// Backend (python/src/app/example/auth.py): a reusable dependency.
async def get_current_user(credentials = Depends(bearer_scheme)) -> User:
    payload = jwt.decode(credentials.credentials, JWT_SECRET, algorithms=[JWT_ALGORITHM])
    ...
    return user

# Protect any route — custom or otherwise — the same way:
@app.get("/api/auth/secret")
async def secret(current_user: User = Depends(get_current_user)):
    return {"message": f"Hello {current_user.username}"}`}</code></pre>
            </bs.Card>
        </div>
    );
}
