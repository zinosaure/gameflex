import React, { useContext, useEffect, useRef, useState } from 'react';
import * as bs from 'react-bootstrap';
import { useSearchParams } from 'react-router-dom';
import Skeleton from "../../skeleton.js";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

// Demo-only: the dev server only proxies /api to the backend, so uploaded images
// (served from the backend's own /src/public mount) are linked to directly here.
const BACKEND_ORIGIN = "http://localhost:10102";
const GENRE_OPTIONS = ["RPG", "Action", "Platformer", "Puzzle", "Adventure", "Simulation", "Sports", "Racing", "Fighting", "Shooter"];
const RELEASE_YEAR_OPTIONS = Array.from({ length: 2030 - 1970 + 1 }, (_, i) => 2030 - i);

async function api(url, options = {}) {
    const res = await fetch(url, options);

    if (res.status === 204) {
        return { state: "success", item: null, ok: true };
    }

    const json = await res.json().catch(() => ({ state: "failed", exception: { detail: "Invalid server response" } }));

    return { ...json, ok: res.ok };
}

function photoUrl(photo) {
    if (!photo) {
        return null;
    }

    return photo.startsWith("/") ? `${BACKEND_ORIGIN}${photo}` : null;
}

// Manages the many-to-many Game <-> Console links for one game, using the
// game-consoles REST endpoint directly (LIST + POST + DELETE).
function ConsoleLinker({ gameId, allConsoles, context }) {
    const [links, setLinks] = useState([]);
    const [selected, setSelected] = useState("");
    const [isPending, setPending] = useState(true);

    const fetchLinks = async () => {
        setPending(true);

        const result = await api(`/api/game-consoles?items_per_page=50&search=${encodeURIComponent(JSON.stringify({ game_id: gameId }))}`);

        setLinks(result.items || []);
        setPending(false);
    };

    useEffect(() => { fetchLinks(); }, [gameId]);

    const linkConsole = async () => {
        if (!selected) {
            return;
        }

        const result = await api("/api/game-consoles", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ game_id: gameId, console_id: selected }),
        });

        if (result.ok) {
            setSelected("");
            fetchLinks();
        } else {
            context.toast.open(result.exception?.detail || "Could not link console", { variant: "error" });
        }
    };

    const unlink = async (linkId) => {
        const result = await api(`/api/game-consoles/${linkId}`, { method: "DELETE" });

        if (result.ok) {
            fetchLinks();
        } else {
            context.toast.open("Could not unlink console", { variant: "error" });
        }
    };

    const linkedIds = links.map(link => link.console_id);
    const availableConsoles = allConsoles.filter(console => !linkedIds.includes(console.id));
    const linkerContext = { values: { console: selected }, setValues: (values) => setSelected(values.console || "") };

    return (
        <div>
            <div className="d-flex flex-wrap gap-2 mb-2">
                {isPending ? <bs.Spinner size="sm" animation="border" /> : links.length === 0 ? (
                    <span className="text-muted small">No consoles linked yet.</span>
                ) : links.map(link => (
                    <bs.Badge key={link.id} bg="secondary" className="d-flex align-items-center gap-2 py-2">
                        {allConsoles.find(console => console.id === link.console_id)?.name || link.console_id}
                        <i className="bi bi-x-circle" role="button" tabIndex={0} onClick={() => unlink(link.id)} onKeyDown={Skeleton.keyActivate(() => unlink(link.id))}></i>
                    </bs.Badge>
                ))}
            </div>
            <div className="d-flex gap-2 align-items-start">
                <div className="flex-grow-1">
                    <Form.Select
                        name="console"
                        context={linkerContext}
                        options={[{ value: "", label: "Link a console…" }, ...availableConsoles.map(console => ({ value: console.id, label: console.name }))]}
                        props={{ size: "sm" }}
                    />
                </div>
                <bs.Button size="sm" variant="outline-primary" disabled={!selected} onClick={linkConsole}>Link</bs.Button>
            </div>
        </div>
    );
}

// Create/edit panel — a real Skeleton.Form posting multipart/form-data straight
// to the games REST endpoint (POST for create, PATCH for edit), including files.
function GameFormPanel({ context, game = null, allConsoles, onSaved }) {
    const [values, setValues] = useState({
        publisher: game?.publisher || "",
        title: game?.title || "",
        license: game?.license || "",
        description: game?.description || "",
        release_date: game?.release_date || "",
        genre: game?.genre || [],
    });
    const [files, setFiles] = useState({});
    const [isPending, setPending] = useState(false);
    const rulesets = {
        publisher: (data) => (!data.publisher || data.publisher.length < 2) ? Ruleset.isInvalid("Publisher is required") : null,
        title: (data) => (!data.title || data.title.length < 2) ? Ruleset.isInvalid("Title is required") : null,
    };
    const formContext = { values, setValues, files, setFiles, rulesets };
    const isValid = Ruleset.hasFeedbacks(formContext) === null;

    const handleSave = async () => {
        setPending(true);

        const formData = new FormData();

        Object.entries(values).forEach(([name, value]) => {
            // "photo"/"path" are handled below via `files` — Form.File writes its own
            // preview bookkeeping ({name, pValue} entries) into values[name] as soon as
            // it renders, even when no file was picked, so this generic loop must not
            // forward those two keys or it submits a bogus "[]"/stringified preview.
            if (name === "photo" || name === "path") {
                return;
            }

            if (value === null || value === undefined || value === "") {
                return;
            }

            formData.append(name, typeof value === "object" ? JSON.stringify(value) : value);
        });

        if (files.photo?.[0]) {
            formData.append("photo", files.photo[0]);
        }

        if (files.path?.[0]) {
            formData.append("path", files.path[0]);
        }

        const result = await api(game ? `/api/games/${game.id}` : "/api/games", {
            method: game ? "PATCH" : "POST",
            body: formData,
        });

        setPending(false);

        if (result.ok) {
            context.toast.open(`"${values.title}" ${game ? "updated" : "created"}`, { variant: "success" });
            context.offcanvas.close();
            onSaved();
        } else {
            context.alert(result.exception?.detail?.map?.(d => d.message).join(", ") || result.exception?.detail || "Save failed", { variant: "error", title: "Could not save" });
        }
    };

    return (
        <div>
            <Form>
                <Form.Group label="Publisher"><Form.Text name="publisher" context={formContext} /></Form.Group>
                <Form.Group label="Title"><Form.Text name="title" context={formContext} /></Form.Group>
                <Form.Group label="License" helpText="Franchise/series name"><Form.Text name="license" context={formContext} /></Form.Group>
                <Form.Group label="Release year">
                    <Form.Select
                        name="release_date"
                        context={formContext}
                        options={[{ value: "", label: "—" }, ...RELEASE_YEAR_OPTIONS.map(year => ({ value: year, label: year }))]}
                    />
                </Form.Group>
                <Form.Group label="Genre">
                    <Form.SkeletonList
                        name="genre"
                        context={formContext}
                        inputType="select"
                        options={GENRE_OPTIONS.map(g => ({ value: g, label: g }))}
                    />
                </Form.Group>
                <Form.Group label="Description"><Form.Longtext name="description" context={formContext} props={{ rows: 3 }} /></Form.Group>
                <Form.Group label="Cover photo" helpText="Max 1MB"><Form.File name="photo" context={formContext} accept="image/*" /></Form.Group>
                <Form.Group label="ROM file" helpText="Max 15MB"><Form.File name="path" context={formContext} /></Form.Group>
            </Form>

            {game ? (
                <Form.Group label="Consoles">
                    <ConsoleLinker gameId={game.id} allConsoles={allConsoles} context={context} />
                </Form.Group>
            ) : null}

            <div className="d-flex justify-content-end gap-2 mt-3">
                <bs.Button variant="outline-secondary" onClick={() => context.offcanvas.close()}>Cancel</bs.Button>
                <bs.Button variant="primary" disabled={!isValid || isPending} onClick={handleSave}>
                    {isPending ? <bs.Spinner size="sm" animation="border" /> : (game ? "Save changes" : "Create game")}
                </bs.Button>
            </div>
        </div>
    );
}

const ITEMS_PER_PAGE = 10;
const MIN_PENDING_MS = 500;

export default function LiveGames() {
    const context = useContext(Skeleton.context);
    const [searchParams, setSearchParams] = useSearchParams();
    const initialPage = Math.max(1, parseInt(searchParams.get("page")) || 1);
    // Tracks the page we ourselves last pushed into the URL, so the
    // "sync back from the URL" effect below can tell a real browser
    // back/forward navigation (ref stays stale) apart from a re-render
    // triggered by our own setSearchParams call (ref already matches).
    const currentUrlPageRef = useRef(initialPage);
    const isFirstSortRef = useRef(true);
    const [items, setItems] = useState([]);
    const [pagination, setPagination] = useState({ count: 0, page: initialPage });
    const [isPending, setPending] = useState(true);
    const [searchText, setSearchText] = useState("");
    const [sortField, setSortField] = useState("id");
    const [sortDesc, setSortDesc] = useState(true);
    const [allConsoles, setAllConsoles] = useState([]);

    const fetchGames = async (page = 1) => {
        setPending(true);

        const startedAt = Date.now();
        const params = new URLSearchParams({
            page,
            items_per_page: ITEMS_PER_PAGE,
            include_unbound: 1,
            sorts: JSON.stringify({ [sortField]: sortDesc ? -1 : 1 }),
        });

        if (searchText) {
            params.set("search", JSON.stringify([{ title: { like: searchText } }, { publisher: { like: searchText } }]));
        }

        const result = await api(`/api/games?${params.toString()}`);

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
        // this particular page actually returned (e.g. fewer on a trailing
        // page), not the requested page size — never feed it back into the
        // next request or the pager would shrink to match the last page visited.
        setPagination(result.pagination ? { ...result.pagination, items_per_page: ITEMS_PER_PAGE } : pagination);
        setPending(false);
    };

    const fetchConsoles = async () => {
        const result = await api("/api/consoles?items_per_page=100");
        setAllConsoles(result.items || []);
    };

    // User-driven navigation (pager, search, sort reset): push ?page= into the
    // URL/history *and* fetch. Plain fetchGames() calls (initial load, refresh
    // after edit/delete on the same page, browser back/forward) never push —
    // pushing there would either duplicate the current history entry or fight
    // the navigation that's already in progress.
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

        fetchGames(page);
    };

    useEffect(() => { fetchConsoles(); }, []);
    useEffect(() => { fetchGames(initialPage); }, []);
    useEffect(() => {
        if (isFirstSortRef.current) {
            isFirstSortRef.current = false;
            return;
        }

        goToPage(1);
    }, [sortField, sortDesc]);

    // Browser back/forward changes the URL without us calling goToPage — pick
    // that up here and fetch the page the URL now points to.
    useEffect(() => {
        const page = Math.max(1, parseInt(searchParams.get("page")) || 1);

        if (page !== currentUrlPageRef.current) {
            currentUrlPageRef.current = page;
            fetchGames(page);
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
        context.offcanvas.open(<GameFormPanel context={context} allConsoles={allConsoles} onSaved={() => goToPage(1)} />, { title: "New game", size: 480 });
    };

    const openEdit = async (game) => {
        const result = await api(`/api/games/${game.id}`);

        if (!result.ok) {
            context.toast.open("Could not load this game", { variant: "error" });
            return;
        }

        context.offcanvas.open(<GameFormPanel context={context} game={result.item} allConsoles={allConsoles} onSaved={() => fetchGames(pagination.page)} />, { title: `Edit: ${game.title}`, size: 480 });
    };

    const confirmDelete = (game) => {
        context.modal.open(`Delete "${game.title}"? This cannot be undone.`, {
            variant: "confirm",
            title: "Delete game",
            confirmLabel: "Delete",
            cancelLabel: "Cancel",
            onConfirm: async () => {
                const result = await api(`/api/games/${game.id}`, { method: "DELETE" });

                if (result.ok) {
                    context.toast.open(`"${game.title}" deleted`, { variant: "success" });
                    fetchGames(pagination.page);
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
                        fetchConsoles();
                        goToPage(1);
                    } else {
                        context.toast.open("Reset failed", { variant: "error" });
                    }
                },
            }
        );
    };

    // Filter/sort inputs are Skeleton.Form components too — they don't need
    // validation rulesets, just a { values, setValues } context to read/write.
    const filterContext = {
        values: { search: searchText, sort: sortField },
        setValues: (values) => {
            setSearchText(values.search || "");
            setSortField(values.sort || "id");
        },
    };

    return (
        <div className="container">
            <div className="mb-4 d-flex justify-content-between align-items-start flex-wrap gap-2">
                <div>
                    <h1 className="display-5 fw-bold mb-3">Live Games</h1>
                    <p className="lead text-muted mb-0">
                        A fully working CRUD screen wired to the real backend example (<code>/api/games</code>, <code>/api/consoles</code>, <code>/api/game-consoles</code>) —
                        listing, search, sort, pagination, create/edit with file upload, delete, and many-to-many relation management.
                    </p>
                </div>
                <bs.Button variant="outline-danger" size="sm" onClick={confirmResetDatabase} className="text-nowrap">
                    <i className="bi bi-arrow-counterclockwise me-1"></i>Reset demo data
                </bs.Button>
            </div>

            <bs.Row className="g-2 mb-3 align-items-center">
                <bs.Col md={5}>
                    <div className="d-flex gap-2">
                        <div className="flex-grow-1">
                            <Form.SearchText
                                name="search"
                                context={filterContext}
                                props={{ placeholder: "Search title or publisher…", onKeyDown: (e) => e.key === "Enter" && goToPage(1) }}
                            />
                        </div>
                        <bs.Button variant="outline-secondary" onClick={() => goToPage(1)}><i className="bi bi-search"></i></bs.Button>
                    </div>
                </bs.Col>
                <bs.Col md={4} className="d-flex gap-2">
                    <div className="flex-grow-1">
                        <Form.Select
                            name="sort"
                            context={filterContext}
                            options={[
                                { value: "id", label: "Sort: ID" },
                                { value: "title", label: "Sort: Title" },
                                { value: "release_date", label: "Sort: Release year" },
                            ]}
                        />
                    </div>
                    <bs.Button variant="outline-secondary" onClick={() => setSortDesc(!sortDesc)}>
                        <i className={`bi bi-sort-${sortDesc ? "down" : "up"}`}></i>
                    </bs.Button>
                </bs.Col>
                <bs.Col md={3} className="text-end">
                    <bs.Button variant="primary" onClick={openCreate}><i className="bi bi-plus-lg me-1"></i>New game</bs.Button>
                </bs.Col>
            </bs.Row>

            <Skeleton.Table
                isPending={isPending}
                itemsPerPage={ITEMS_PER_PAGE}
                itemCount={pagination.count}
                labels={{
                    photo: {
                        title: "",
                        css: { width: "56px" },
                        placeholder: () => <div className="placeholder rounded" style={{ width: 40, height: 40 }}></div>,
                        callback: (item) => photoUrl(item.photo) ? (
                            <img src={photoUrl(item.photo)} alt="" style={{ width: 40, height: 40, objectFit: "cover", borderRadius: 6 }} />
                        ) : (
                            <div className="d-flex align-items-center justify-content-center bg-body-secondary rounded" style={{ width: 40, height: 40 }}>
                                <i className="bi bi-controller text-muted"></i>
                            </div>
                        ),
                    },
                    title: {
                        title: "Title",
                        // Wrapped in the exact same classes as the real line below
                        // (not just a hardcoded height) — a bare .placeholder span's
                        // own height tracks raw font-size, not line-height, so it
                        // renders shorter than the actual text line it's replacing.
                        placeholder: () => (
                            <div>
                                <div className="fw-semibold"><span className="placeholder col-8"></span></div>
                                <div className="text-muted small"><span className="placeholder col-5"></span></div>
                            </div>
                        ),
                        callback: (item) => (
                            <div>
                                <div className="fw-semibold">{item.title}</div>
                                <div className="text-muted small">{item.publisher}{item.release_date ? ` · ${item.release_date}` : ""}</div>
                            </div>
                        ),
                    },
                    genre: {
                        title: "Genre",
                        callback: (item) => (item.genre || []).map(g => <bs.Badge key={g} bg="info" className="me-1">{g}</bs.Badge>),
                    },
                    consoles: {
                        title: "Consoles",
                        callback: (item) => item.count_consoles > 0 ? (
                            <bs.Badge bg="secondary">{item.count_consoles} linked</bs.Badge>
                        ) : <span className="text-muted small">none</span>,
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
