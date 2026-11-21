import React, { useContext, useEffect, useRef, useState } from 'react';
import * as bs from 'react-bootstrap';
import { useSearchParams } from 'react-router-dom';
import Skeleton from "../../skeleton.js";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

async function api(url, options = {}) {
    const res = await fetch(url, options);

    if (res.status === 204) {
        return { state: "success", item: null, ok: true };
    }

    const json = await res.json().catch(() => ({ state: "failed", exception: { detail: "Invalid server response" } }));

    return { ...json, ok: res.ok };
}

// Create/edit panel — posts JSON straight to the consoles REST endpoint.
// "id" is the primary key here (a user-chosen slug, e.g. "snes"), so it's
// only editable at creation time.
function ConsoleFormPanel({ context, console_ = null, onSaved }) {
    const [values, setValues] = useState({
        id: console_?.id || "",
        name: console_?.name || "",
        short_name: console_?.short_name || "",
        core: console_?.core || "",
        manufacturer: console_?.manufacturer || "",
        release_date: console_?.release_date || "",
    });
    const [isPending, setPending] = useState(false);
    const rulesets = {
        id: (data) => (!data.id || data.id.length < 2) ? Ruleset.isInvalid("A short id is required (e.g. \"snes\")") : null,
        name: (data) => (!data.name || data.name.length < 2) ? Ruleset.isInvalid("Name is required") : null,
        core: (data) => (!data.core) ? Ruleset.isInvalid("Emulator core is required") : null,
    };
    const formContext = { values, setValues, files: {}, setFiles: () => { }, rulesets };
    const isValid = Ruleset.hasFeedbacks(formContext) === null;

    const handleSave = async () => {
        setPending(true);

        const payload = {};

        Object.entries(values).forEach(([name, value]) => {
            if (value !== "" && value !== null && value !== undefined) {
                payload[name] = value;
            }
        });

        const result = await api(console_ ? `/api/consoles/${console_.id}` : "/api/consoles", {
            method: console_ ? "PATCH" : "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(console_ ? { ...payload, id: console_.id } : payload),
        });

        setPending(false);

        if (result.ok) {
            context.toast.open(`"${values.name}" ${console_ ? "updated" : "created"}`, { variant: "success" });
            context.offcanvas.close();
            onSaved();
        } else {
            context.alert(result.exception?.detail?.map?.(d => d.message).join(", ") || result.exception?.detail || "Save failed", { variant: "error", title: "Could not save" });
        }
    };

    return (
        <div>
            <Form>
                <Form.Group label="ID" helpText="Short slug, e.g. &quot;snes&quot; — cannot be changed later">
                    <Form.Text name="id" context={formContext} props={{ autoComplete: "off", disabled: !!console_ }} />
                </Form.Group>
                <Form.Group label="Name"><Form.Text name="name" context={formContext} /></Form.Group>
                <Form.Group label="Short name"><Form.Text name="short_name" context={formContext} /></Form.Group>
                <Form.Group label="Emulator core" helpText="e.g. &quot;snes9x&quot;"><Form.Text name="core" context={formContext} /></Form.Group>
                <Form.Group label="Manufacturer"><Form.Text name="manufacturer" context={formContext} /></Form.Group>
                <Form.Group label="Release year"><Form.IntFloat name="release_date" context={formContext} min={1970} max={2030} step={1} /></Form.Group>
            </Form>

            <div className="d-flex justify-content-end gap-2 mt-3">
                <bs.Button variant="outline-secondary" onClick={() => context.offcanvas.close()}>Cancel</bs.Button>
                <bs.Button variant="primary" disabled={!isValid || isPending} onClick={handleSave}>
                    {isPending ? <bs.Spinner size="sm" animation="border" /> : (console_ ? "Save changes" : "Create console")}
                </bs.Button>
            </div>
        </div>
    );
}

const ITEMS_PER_PAGE = 10;
const MIN_PENDING_MS = 500;

export default function LiveConsoles() {
    const context = useContext(Skeleton.context);
    const [searchParams, setSearchParams] = useSearchParams();
    const initialPage = Math.max(1, parseInt(searchParams.get("page")) || 1);
    // Tracks the page we ourselves last pushed into the URL, so the
    // "sync back from the URL" effect below can tell a real browser
    // back/forward navigation (ref stays stale) apart from a re-render
    // triggered by our own setSearchParams call (ref already matches).
    const currentUrlPageRef = useRef(initialPage);
    const [items, setItems] = useState([]);
    const [pagination, setPagination] = useState({ count: 0, page: initialPage });
    const [isPending, setPending] = useState(true);
    const [searchText, setSearchText] = useState("");

    const fetchConsoles = async (page = 1) => {
        setPending(true);

        const startedAt = Date.now();
        const params = new URLSearchParams({ page, items_per_page: ITEMS_PER_PAGE, include_unbound: 1 });

        if (searchText) {
            params.set("search", JSON.stringify([{ name: { like: searchText } }, { manufacturer: { like: searchText } }]));
        }

        const result = await api(`/api/consoles?${params.toString()}`);

        // Demo-only: on localhost this resolves in a few ms, too fast to actually
        // see the placeholder rows this page demonstrates. Pad out to a minimum
        // visible duration rather than dropping the delay somewhere less honest
        // (like a fake setTimeout instead of a real fetch).
        const elapsed = Date.now() - startedAt;

        if (elapsed < MIN_PENDING_MS) {
            await new Promise((resolve) => setTimeout(resolve, MIN_PENDING_MS - elapsed));
        }

        setItems(result.items || []);
        // The backend's own pagination.items_per_page reflects how many items
        // this particular page actually returned (e.g. 3 on a trailing page),
        // not the requested page size — never feed it back into the next
        // request or the pager would shrink to match the last page visited.
        setPagination(result.pagination ? { ...result.pagination, items_per_page: ITEMS_PER_PAGE } : pagination);
        setPending(false);
    };

    // User-driven navigation (pager, search reset): push ?page= into the
    // URL/history *and* fetch. Plain fetchConsoles() calls (initial load,
    // refresh after edit/delete on the same page, browser back/forward)
    // never push — pushing there would either duplicate the current history
    // entry or fight the navigation that's already in progress.
    const goToPage = (page) => {
        currentUrlPageRef.current = page;

        if ((parseInt(searchParams.get("page")) || 1) !== page) {
            setSearchParams(prev => {
                const next = new URLSearchParams(prev);

                if (page > 1) {
                    next.set("page", page);
                } else {
                    next.delete("page");
                }

                return next;
            });
        }

        fetchConsoles(page);
    };

    useEffect(() => { fetchConsoles(initialPage); }, []);

    // Browser back/forward changes the URL without us calling goToPage — pick
    // that up here and fetch the page the URL now points to.
    useEffect(() => {
        const page = Math.max(1, parseInt(searchParams.get("page")) || 1);

        if (page !== currentUrlPageRef.current) {
            currentUrlPageRef.current = page;
            fetchConsoles(page);
        }
    }, [searchParams]);

    const pager = Skeleton.usePagination({
        isPending,
        currentPosition: pagination.page,
        itemCount: pagination.count,
        itemsPerPage: ITEMS_PER_PAGE,
        onPositionChange: (page) => goToPage(page),
    });

    const openCreate = () => {
        context.offcanvas.open(<ConsoleFormPanel context={context} onSaved={() => goToPage(1)} />, { title: "New console", size: 420 });
    };

    const openEdit = (console_) => {
        context.offcanvas.open(<ConsoleFormPanel context={context} console_={console_} onSaved={() => fetchConsoles(pagination.page)} />, { title: `Edit: ${console_.name}`, size: 420 });
    };

    const confirmDelete = (console_) => {
        context.modal.open(`Delete "${console_.name}"? Games linked to it will lose this console.`, {
            variant: "confirm",
            title: "Delete console",
            confirmLabel: "Delete",
            cancelLabel: "Cancel",
            onConfirm: async () => {
                const result = await api(`/api/consoles/${console_.id}`, { method: "DELETE" });

                if (result.ok) {
                    context.toast.open(`"${console_.name}" deleted`, { variant: "success" });
                    fetchConsoles(pagination.page);
                } else {
                    context.toast.open("Delete failed", { variant: "error" });
                }
            },
        });
    };

    // Demo-only: this in-memory backend has no persistence,
    // so this is the only way to undo whatever visitors have created/edited/deleted.
    const confirmResetDatabase = () => {
        context.modal.open(
            "This resets the entire demo dataset (games, consoles, and their links) back to the original seed data. Anything you created, edited, or uploaded will be lost.",
            {
                variant: "confirm",
                title: "Reset demo database",
                confirmLabel: "Reset",
                cancelLabel: "Cancel",
                onConfirm: async () => {
                    const result = await api("/api/retrogaming/reset", { method: "POST" });

                    if (result.ok) {
                        context.toast.open("Demo database reset", { variant: "success" });
                        goToPage(1);
                    } else {
                        context.toast.open("Reset failed", { variant: "error" });
                    }
                },
            }
        );
    };

    // The search input is a Skeleton.Form component too — it doesn't need
    // validation rulesets, just a { values, setValues } context to read/write.
    const filterContext = {
        values: { search: searchText },
        setValues: (values) => setSearchText(values.search || ""),
    };

    return (
        <div className="container">
            <div className="mb-4 d-flex justify-content-between align-items-start flex-wrap gap-2">
                <div>
                    <h1 className="display-5 fw-bold mb-3">Live Consoles</h1>
                    <p className="lead text-muted mb-0">
                        A second screen against the same backend example (<code>/api/consoles</code>) — a simpler CRUD without file upload, complementing the Games screen.
                    </p>
                </div>
                <bs.Button variant="outline-danger" size="sm" onClick={confirmResetDatabase} className="text-nowrap">
                    <i className="bi bi-arrow-counterclockwise me-1"></i>Reset demo data
                </bs.Button>
            </div>

            <bs.Row className="g-2 mb-3 align-items-center">
                <bs.Col md={8}>
                    <div className="d-flex gap-2">
                        <div className="flex-grow-1">
                            <Form.SearchText
                                name="search"
                                context={filterContext}
                                props={{ placeholder: "Search name or manufacturer…", onKeyDown: (e) => e.key === "Enter" && goToPage(1) }}
                            />
                        </div>
                        <bs.Button variant="outline-secondary" onClick={() => goToPage(1)}><i className="bi bi-search"></i></bs.Button>
                    </div>
                </bs.Col>
                <bs.Col md={4} className="text-end">
                    <bs.Button variant="primary" onClick={openCreate}><i className="bi bi-plus-lg me-1"></i>New console</bs.Button>
                </bs.Col>
            </bs.Row>

            <Skeleton.Table
                isPending={isPending}
                itemsPerPage={ITEMS_PER_PAGE}
                itemCount={pagination.count}
                labels={{
                    name: {
                        title: "Console",
                        // Wrapped in the exact same classes as the real line below
                        // (not just a hardcoded height) — a bare .placeholder span's
                        // own height tracks raw font-size, not line-height, so it
                        // renders shorter than the actual text line it's replacing.
                        // Matching the wrapper classes keeps this correct even if
                        // the base font scale changes later.
                        placeholder: () => (
                            <div>
                                <div className="fw-semibold"><span className="placeholder col-8"></span></div>
                                <div className="text-muted small"><span className="placeholder col-5"></span></div>
                            </div>
                        ),
                        callback: (item) => (
                            <div>
                                <div className="fw-semibold">{item.name}</div>
                                <div className="text-muted small">{item.id} · {item.manufacturer}{item.release_date ? ` · ${item.release_date}` : ""}</div>
                            </div>
                        ),
                    },
                    core: { title: "Core", callback: "core" },
                    games: {
                        title: "Games",
                        callback: (item) => item.count_games > 0 ? <bs.Badge bg="secondary">{item.count_games}</bs.Badge> : <span className="text-muted small">0</span>,
                    },
                    edit: {
                        title: "",
                        css: { width: "40px", textAlign: "center" },
                        callback: (item) => <i className="bi bi-pencil text-primary" role="button" tabIndex={0} onClick={() => openEdit(item)} onKeyDown={Skeleton.keyActivate(() => openEdit(item))}></i>,
                    },
                    delete: {
                        title: "",
                        css: { width: "40px", textAlign: "center" },
                        callback: (item) => <i className="bi bi-trash text-danger" role="button" tabIndex={0} onClick={() => confirmDelete(item)} onKeyDown={Skeleton.keyActivate(() => confirmDelete(item))}></i>,
                    },
                }}
                items={items}
            />

            <pager.PrevNext>
                <pager.Selectbox />
            </pager.PrevNext>
        </div>
    );
}
