import React, { useContext, useState } from 'react';
import * as bs from 'react-bootstrap';
import Skeleton from "../skeleton.js";


const TOAST_VARIANTS = ["normal", "info", "success", "warning", "error", "confirm"];
const TOAST_POSITIONS = ["top-start", "top-center", "top-end", "middle-start", "middle-center", "middle-end", "bottom-start", "bottom-center", "bottom-end"];
const ALERT_VARIANTS = ["normal", "info", "success", "warning", "error"];

function CodeBlock({ title = "Code", children }) {
    return (
        <bs.Accordion className="mb-0">
            <bs.Accordion.Item eventKey="0">
                <bs.Accordion.Header><i className="bi bi-code-slash me-2"></i>{title}</bs.Accordion.Header>
                <bs.Accordion.Body className="bg-body-tertiary p-3">
                    <pre className="mb-0"><code style={{ fontSize: "0.875rem" }}>{children}</code></pre>
                </bs.Accordion.Body>
            </bs.Accordion.Item>
        </bs.Accordion>
    );
}

export default function FeedbacksDoc() {
    const context = useContext(Skeleton.context);

    const NestedLevel = ({ level = 1 }) => (
        <div>
            <p>You are at nesting level <bs.Badge bg="dark">{level}</bs.Badge>. Each one is opened from a button inside the previous one's own content.</p>
            <bs.Button size="sm" variant="primary" onClick={() => context.modal.open(<NestedLevel level={level + 1} />, { title: `Level ${level + 1}`, size: level % 2 ? "sm" : null })}>
                Open level {level + 1} from here <i className="bi bi-arrow-right"></i>
            </bs.Button>
        </div>
    );

    const EditForm = ({ onSaved }) => {
        const [values, setValues] = useState({ title: "" });
        const Ruleset = Skeleton.Ruleset;
        const rulesets = {
            title: (data) => (!data.title || data.title.length < 3) ? Ruleset.isInvalid("Title must be at least 3 characters") : null,
        };
        const context_ = { values, setValues, files: {}, setFiles: () => { }, rulesets };
        const isValid = Ruleset.hasFeedbacks(context_) === null;

        return (
            <div>
                <Skeleton.Form>
                    <Skeleton.Form.Group label="Title" helpText="At least 3 characters">
                        <Skeleton.Form.Text name="title" context={context_} />
                    </Skeleton.Form.Group>
                </Skeleton.Form>
                <div className="d-flex justify-content-end gap-2">
                    <bs.Button variant="outline-secondary" onClick={() => context.modal.close()}>Cancel</bs.Button>
                    <bs.Button variant="primary" disabled={!isValid} onClick={() => { onSaved(values.title); context.modal.close(); }}>Save</bs.Button>
                </div>
            </div>
        );
    };

    const LongContent = () => (
        <div>
            {Array.from({ length: 25 }, (_, i) => (
                <p key={i}>Paragraph {i + 1} — the modal grows with its content and the page scrolls behind it once it exceeds the viewport height.</p>
            ))}
        </div>
    );

    return (
        <div className="container">
            <div className="mb-5">
                <h1 className="display-5 fw-bold mb-3">Feedbacks</h1>
                <p className="lead text-muted">
                    Toasts, alerts and modals — corner notifications, dismissible banners, and centered dialogs. All three share the same variant/icon system and the same <code>onConfirm</code>/<code>onCancel</code> contract.
                </p>
            </div>

            <bs.Tabs defaultActiveKey="toast" id="feedbacks-tabs" className="nav-tabs-boxed mb-4" justify>
                <bs.Tab eventKey="toast" label="Toast">
                    <div>
                        <h2 className="display-5 fw-bold mb-3">Toast</h2>
                        <p className="lead text-muted">
                            Corner notifications built on the native Bootstrap Toast component — positions, variants, duration and content types.
                        </p>

                        {/* Toast Example 1: Variants */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">1</bs.Badge>Variants</h2>
                            <p>Six variants are available: <code>normal</code>, <code>info</code>, <code>success</code>, <code>warning</code>, <code>error</code>, <code>confirm</code>. <code>confirm</code> replaces the close button with Cancel/Confirm — see section 7 below for the full callback-driven flow.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <div className="d-flex gap-2 flex-wrap">
                                    {TOAST_VARIANTS.map(variant => (
                                        <bs.Button
                                            key={variant}
                                            variant="outline-primary"
                                            onClick={() => context.toast.open(`This is a "${variant}" toast`, {
                                                variant,
                                                title: variant,
                                                ...(variant === "confirm" ? {
                                                    onConfirm: () => context.toast.open("Confirmed!", { variant: "success" }),
                                                    onCancel: () => context.toast.open("Cancelled", { variant: "normal" }),
                                                } : {}),
                                            })}
                                        >
                                            {variant}
                                        </bs.Button>
                                    ))}
                                </div>
                            </div>

                            <CodeBlock>{`context.toast.open("This is a \\"success\\" toast", { variant: "success", title: "success" });

// "confirm" wires onConfirm/onCancel to a follow-up toast:
context.toast.open('This is a "confirm" toast', {
    variant: "confirm",
    title: "confirm",
    onConfirm: () => context.toast.open("Confirmed!", { variant: "success" }),
    onCancel: () => context.toast.open("Cancelled", { variant: "normal" }),
});`}</CodeBlock>
                        </div>

                        {/* Toast Example 2: Positions */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">2</bs.Badge>Positions</h2>
                            <p>All 9 standard corner/edge combinations are supported. Default is <code>top-end</code>.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <div className="d-flex gap-2 flex-wrap">
                                    {TOAST_POSITIONS.map(position => (
                                        <bs.Button key={position} size="sm" variant="outline-secondary" onClick={() => context.toast.open(`Position: ${position}`, { position })}>
                                            {position}
                                        </bs.Button>
                                    ))}
                                </div>
                            </div>

                            <CodeBlock>{`context.toast.open("Position: bottom-start", { position: "bottom-start" });`}</CodeBlock>
                        </div>

                        {/* Toast Example 3: Duration */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">3</bs.Badge>Duration</h2>
                            <p><code>duration</code> is the autohide delay in milliseconds (default <code>5000</code>). Use <code>0</code> for a persistent toast (closed only via the close button or <code>toast.close(id)</code>).</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <div className="d-flex gap-2 flex-wrap">
                                    <bs.Button variant="outline-primary" onClick={() => context.toast.open("Gone in 1.5s", { duration: 1500, title: "Fast" })}>
                                        1.5s
                                    </bs.Button>
                                    <bs.Button variant="outline-primary" onClick={() => context.toast.open("Stays until dismissed", { duration: 0, title: "Persistent" })}>
                                        Persistent (duration: 0)
                                    </bs.Button>
                                </div>
                            </div>

                            <CodeBlock>{`context.toast.open("Stays until dismissed", { duration: 0 });`}</CodeBlock>
                        </div>

                        {/* Toast Example 4: Content types */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">4</bs.Badge>Content: text, HTML or React</h2>
                            <p>The content argument can be a plain string, an HTML string, or a React element (portal-rendered, keeps full access to context/router).</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <div className="d-flex gap-2 flex-wrap">
                                    <bs.Button variant="outline-secondary" onClick={() => context.toast.open("Plain text content")}>
                                        Text
                                    </bs.Button>
                                    <bs.Button variant="outline-secondary" onClick={() => context.toast.open("<b>Bold</b> <i>HTML</i> content")}>
                                        HTML
                                    </bs.Button>
                                    <bs.Button variant="outline-secondary" onClick={() => context.toast.open(<div>React content <bs.Spinner size="sm" animation="border" /></div>)}>
                                        React element
                                    </bs.Button>
                                </div>
                            </div>

                            <CodeBlock>{`context.toast.open("Plain text content");
context.toast.open("<b>Bold</b> <i>HTML</i> content");
context.toast.open(<div>React content <Spinner size="sm" animation="border" /></div>);`}</CodeBlock>
                        </div>

                        {/* Toast Example 5: without title */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">5</bs.Badge>Without a title</h2>
                            <p>Omitting <code>title</code> renders the compact header-less layout (Bootstrap's documented pattern).</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <bs.Button variant="outline-dark" onClick={() => context.toast.open("No header on this one", { variant: "success" })}>
                                    Toast without title
                                </bs.Button>
                            </div>

                            <CodeBlock>{`context.toast.open("No header on this one", { variant: "success" }); // no title option`}</CodeBlock>
                        </div>

                        {/* Toast Example 6: Action button (Undo pattern) */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">6</bs.Badge>An action button inside the toast</h2>
                            <p>Since <code>content</code> can be a React element, a toast can carry its own interactive button — here, a classic "Undo" pattern that closes itself via the id returned by <code>toast.open</code>.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <bs.Button
                                    variant="outline-dark"
                                    onClick={() => {
                                        let id;
                                        id = context.toast.open(
                                            <div className="d-flex align-items-center justify-content-between gap-3">
                                                <span>Item archived.</span>
                                                <bs.Button size="sm" variant="light" onClick={() => { context.toast.close(id); context.toast.open("Undone", { variant: "success" }); }}>
                                                    Undo
                                                </bs.Button>
                                            </div>,
                                            { variant: "info", duration: 6000 }
                                        );
                                    }}
                                >
                                    Archive item
                                </bs.Button>
                            </div>

                            <CodeBlock>{`let id;
id = context.toast.open(
    <div className="d-flex align-items-center justify-content-between gap-3">
        <span>Item archived.</span>
        <button onClick={() => { context.toast.close(id); context.toast.open("Undone", { variant: "success" }); }}>
            Undo
        </button>
    </div>,
    { variant: "info", duration: 6000 }
);`}</CodeBlock>
                        </div>

                        {/* Toast Example 7: Confirm */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">7</bs.Badge>Confirm dialog</h2>
                            <p>
                                <code>variant: "confirm"</code> replaces the close button with a split Cancel/Confirm footer, wired to the <code>onConfirm</code>/<code>onCancel</code> callbacks (not a Promise) — same contract as <code>context.modal.open</code>. It never autohides, so a decision can't be dismissed by accident.
                            </p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <bs.Button
                                    variant="danger"
                                    onClick={() => context.toast.open("This action cannot be undone.", {
                                        variant: "confirm",
                                        title: "Delete item?",
                                        confirmLabel: "Delete",
                                        cancelLabel: "Keep it",
                                        onConfirm: () => context.toast.open("Item deleted", { variant: "success" }),
                                        onCancel: () => context.toast.open("Cancelled", { variant: "normal" }),
                                    })}
                                >
                                    Delete item…
                                </bs.Button>
                            </div>

                            <CodeBlock>{`context.toast.open("This action cannot be undone.", {
    variant: "confirm",
    title: "Delete item?",
    confirmLabel: "Delete",
    cancelLabel: "Keep it",
    onConfirm: () => context.toast.open("Item deleted", { variant: "success" }),
    onCancel: () => context.toast.open("Cancelled", { variant: "normal" }),
});`}</CodeBlock>
                        </div>

                        {/* Toast Example 8: Async workflow */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">8</bs.Badge>Async workflow — loading → success/error</h2>
                            <p>Open a persistent "loading" toast, keep its id, then swap it for a success or error toast once the request settles.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <div className="d-flex gap-2 flex-wrap">
                                    <bs.Button
                                        variant="outline-success"
                                        onClick={() => {
                                            const id = context.toast.open(<span><bs.Spinner size="sm" animation="border" className="me-2" />Saving…</span>, { duration: 0, variant: "normal" });
                                            setTimeout(() => {
                                                context.toast.close(id);
                                                context.toast.open("Saved successfully", { variant: "success", title: "Success" });
                                            }, 1500);
                                        }}
                                    >
                                        Simulate a successful save
                                    </bs.Button>
                                    <bs.Button
                                        variant="outline-danger"
                                        onClick={() => {
                                            const id = context.toast.open(<span><bs.Spinner size="sm" animation="border" className="me-2" />Saving…</span>, { duration: 0, variant: "normal" });
                                            setTimeout(() => {
                                                context.toast.close(id);
                                                context.toast.open("Could not save — network error", { variant: "error", title: "Error" });
                                            }, 1500);
                                        }}
                                    >
                                        Simulate a failed save
                                    </bs.Button>
                                </div>
                            </div>

                            <CodeBlock>{`const id = context.toast.open(<Spinner size="sm" />, { duration: 0 });
saveToServer().then(() => {
    context.toast.close(id);
    context.toast.open("Saved successfully", { variant: "success" });
}).catch(() => {
    context.toast.close(id);
    context.toast.open("Could not save", { variant: "error" });
});`}</CodeBlock>
                        </div>

                        {/* Toast Example 9: Multiple toasts stacking */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">9</bs.Badge>Multiple toasts stacking</h2>
                            <p>Several toasts at the same position stack vertically on their own — no manual offset or z-index math needed.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <bs.Button
                                    variant="dark"
                                    onClick={() => {
                                        ["normal", "info", "success", "warning", "error"].forEach((variant, i) => {
                                            setTimeout(() => context.toast.open(`Toast #${i + 1}`, { variant, title: variant }), i * 200);
                                        });
                                    }}
                                >
                                    Fire 5 toasts
                                </bs.Button>
                            </div>

                            <CodeBlock>{`["normal", "info", "success", "warning", "error"].forEach((variant, i) => {
    setTimeout(() => context.toast.open(\`Toast #\${i + 1}\`, { variant }), i * 200);
});`}</CodeBlock>
                        </div>

                        {/* Toast API Reference */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="secondary" className="me-2">API</bs.Badge>Toast — API Reference</h2>

                            <h6 className="mt-4 mb-2"><code>context.toast.open(content, options)</code></h6>
                            <div className="table-responsive mb-4">
                                <table className="table table-bordered">
                                    <thead>
                                        <tr>
                                            <th style={{ width: "20%" }}>Option</th>
                                            <th style={{ width: "20%" }}>Type / default</th>
                                            <th>Description</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        <tr><td><code>content</code></td><td>string | React element</td><td>Text, HTML string, or React element.</td></tr>
                                        <tr><td><code>position</code></td><td>string, default <code>"top-end"</code></td><td>One of the 9 combinations of top/middle/bottom × start/center/end.</td></tr>
                                        <tr><td><code>duration</code></td><td>number (ms), default <code>5000</code></td><td><code>0</code> disables autohide. Ignored (forced to <code>0</code>) when <code>variant: "confirm"</code>.</td></tr>
                                        <tr><td><code>variant</code></td><td>string, default <code>"normal"</code></td><td><code>normal</code> | <code>info</code> | <code>success</code> | <code>warning</code> | <code>error</code> | <code>confirm</code></td></tr>
                                        <tr><td><code>title</code></td><td>string</td><td>Optional bold title above the message.</td></tr>
                                        <tr><td><code>onConfirm</code> / <code>onCancel</code></td><td>function</td><td>Only used when <code>variant: "confirm"</code>.</td></tr>
                                        <tr><td><code>confirmLabel</code> / <code>cancelLabel</code></td><td>string, default <code>"Confirm"</code> / <code>"Cancel"</code></td><td>Only used when <code>variant: "confirm"</code>.</td></tr>
                                    </tbody>
                                </table>
                            </div>

                            <h6 className="mt-4 mb-2"><code>context.toast.close(id)</code></h6>
                            <p className="text-muted">Closes a specific toast. Multiple toasts at the same position stack automatically — no manual z-index management needed.</p>
                        </div>

                    </div>
                </bs.Tab>
                <bs.Tab eventKey="alert" title="Alert">
                    <div>
                        <h2 className="display-5 fw-bold mb-3">Alert</h2>
                        <p className="lead text-muted">
                            Dismissible banners built on the native Bootstrap Alert component, pinned right below the header.
                        </p>

                        {/* Alert Example 1: Variants */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">1</bs.Badge>Variants</h2>
                            <p>Five variants: <code>normal</code>, <code>info</code>, <code>success</code>, <code>warning</code>, <code>error</code>. Same solid-card look and icon system as the toast (no <code>confirm</code> — alerts are non-blocking).</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <div className="d-flex gap-2 flex-wrap">
                                    {ALERT_VARIANTS.map(variant => (
                                        <bs.Button key={variant} variant="outline-primary" onClick={() => context.alert(`This is a "${variant}" alert.`, { variant, title: variant })}>
                                            {variant}
                                        </bs.Button>
                                    ))}
                                </div>
                            </div>

                            <CodeBlock>{`context.alert('This is a "warning" alert.', { variant: "warning", title: "warning" });`}</CodeBlock>
                        </div>

                        {/* Alert Example 2: Duration */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">2</bs.Badge>Auto-dismiss duration</h2>
                            <p><code>duration</code> is optional (default: stays until manually dismissed). When set, the alert auto-closes after the given number of milliseconds.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <div className="d-flex gap-2 flex-wrap">
                                    <bs.Button variant="outline-primary" onClick={() => context.alert("This will close itself in 3s.", { variant: "info", duration: 3000, title: "Auto-dismiss" })}>
                                        Auto-dismiss (3s)
                                    </bs.Button>
                                    <bs.Button variant="outline-secondary" onClick={() => context.alert("Stays until you dismiss it.", { title: "Manual" })}>
                                        Stays until dismissed
                                    </bs.Button>
                                </div>
                            </div>

                            <CodeBlock>{`context.alert("This will close itself in 3s.", { duration: 3000 });`}</CodeBlock>
                        </div>

                        {/* Alert Example 3: Multiple alerts */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">3</bs.Badge>Multiple alerts</h2>
                            <p>Several alerts stack vertically in the same banner area, most recent at the bottom.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <bs.Button
                                    variant="dark"
                                    onClick={() => {
                                        context.alert("First alert", { variant: "info" });
                                        context.alert("Second alert", { variant: "warning" });
                                        context.alert("Third alert", { variant: "success" });
                                    }}
                                >
                                    Show 3 alerts at once
                                </bs.Button>
                            </div>

                            <CodeBlock>{`context.alert("First alert", { variant: "info" });
context.alert("Second alert", { variant: "warning" });
context.alert("Third alert", { variant: "success" });`}</CodeBlock>
                        </div>

                        {/* Alert Example 4: Rich React content */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">4</bs.Badge>Rich content with an action button</h2>
                            <p>Since <code>content</code> can be a React element, an alert can include its own interactive controls — e.g. a "Retry" button for a failed background task.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <bs.Button
                                    variant="outline-danger"
                                    onClick={() => context.alert(
                                        <div className="d-flex align-items-center justify-content-between gap-3">
                                            <span><strong>Sync failed.</strong> The last background sync could not reach the server.</span>
                                            <bs.Button size="sm" variant="danger" onClick={() => context.toast.open("Retrying…", { variant: "info" })}>
                                                Retry
                                            </bs.Button>
                                        </div>,
                                        { variant: "error" }
                                    )}
                                >
                                    Show alert with action button
                                </bs.Button>
                            </div>

                            <CodeBlock>{`context.alert(
    <div className="d-flex align-items-center justify-content-between gap-3">
        <span>Sync failed.</span>
        <button onClick={retry}>Retry</button>
    </div>,
    { variant: "error" }
);`}</CodeBlock>
                        </div>

                        {/* Alert Example 5: Validation summary */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">5</bs.Badge>Real case: form validation summary</h2>
                            <p>A common pattern — summarize every field error in one alert when a form submission is rejected, instead of (or in addition to) per-field feedback.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <bs.Button
                                    variant="outline-warning"
                                    onClick={() => context.alert(
                                        <div>
                                            <strong>Please fix the following before submitting:</strong>
                                            <ul className="mb-0 mt-1">
                                                <li>Email is required</li>
                                                <li>Password must be at least 8 characters</li>
                                            </ul>
                                        </div>,
                                        { variant: "warning", title: "Validation error" }
                                    )}
                                >
                                    Simulate a rejected submit
                                </bs.Button>
                            </div>

                            <CodeBlock>{`context.alert(
    <ul className="mb-0">{errors.map(e => <li key={e}>{e}</li>)}</ul>,
    { variant: "warning", title: "Validation error" }
);`}</CodeBlock>
                        </div>

                        {/* Alert API Reference */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="secondary" className="me-2">API</bs.Badge>Alert — API Reference</h2>

                            <h6 className="mt-4 mb-2"><code>context.alert(content, options)</code></h6>
                            <div className="table-responsive mb-4">
                                <table className="table table-bordered">
                                    <thead>
                                        <tr>
                                            <th style={{ width: "20%" }}>Option</th>
                                            <th style={{ width: "20%" }}>Type / default</th>
                                            <th>Description</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        <tr><td><code>content</code></td><td>string | React element</td><td>Text, HTML string, or React element.</td></tr>
                                        <tr><td><code>variant</code></td><td>string, default <code>"normal"</code></td><td><code>normal</code> | <code>info</code> | <code>success</code> | <code>warning</code> | <code>error</code> (no <code>confirm</code> — alerts are non-blocking).</td></tr>
                                        <tr><td><code>duration</code></td><td>number (ms), default none</td><td>Auto-dismiss delay; omit to require manual dismissal.</td></tr>
                                        <tr><td><code>title</code></td><td>string</td><td>Optional bold heading above the message.</td></tr>
                                    </tbody>
                                </table>
                            </div>
                            <p className="text-muted">There is no <code>closeAlert</code> — dismiss via the built-in close button, or via <code>duration</code>.</p>
                        </div>

                    </div>
                </bs.Tab>
                <bs.Tab eventKey="modal" title="Modal">
                    <div>
                        <h2 className="display-5 fw-bold mb-3">Modal</h2>
                        <p className="lead text-muted">
                            Centered dialogs built on the native Bootstrap Modal component — sizes, variants, a built-in confirm flow, real nesting, and any content type.
                        </p>

                        {/* Modal Example 1: Sizes */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">1</bs.Badge>Sizes</h2>
                            <p>Always centered (<code>.modal-dialog-centered</code>). <code>size</code> accepts <code>sm</code>, <code>lg</code>, <code>xl</code>, or omit for the default width.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <div className="d-flex gap-2 flex-wrap">
                                    {[null, "sm", "lg", "xl"].map(size => (
                                        <bs.Button key={size || "default"} variant="outline-primary" onClick={() => context.modal.open(<p>This is a {size || "default"}-sized modal.</p>, { size, title: `size: ${size || "default"}` })}>
                                            {size || "default"}
                                        </bs.Button>
                                    ))}
                                </div>
                            </div>

                            <CodeBlock>{`context.modal.open(<p>Content</p>, { size: "lg", title: "size: lg" });`}</CodeBlock>
                        </div>

                        {/* Modal Example 2: Variants */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">2</bs.Badge>Variants</h2>
                            <p>SweetAlert2-style: <code>info</code>/<code>success</code>/<code>warning</code>/<code>error</code> show a big colored icon and a centered "OK" footer button. <code>normal</code> has no icon and keeps its natural, non-centered layout — meant for arbitrary content (forms, nested modals) rather than short alert messages.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <div className="d-flex gap-2 flex-wrap">
                                    {["normal", "info", "success", "warning", "error"].map(variant => (
                                        <bs.Button key={variant} variant="outline-primary" onClick={() => context.modal.open(<p>A "{variant}" modal.</p>, { variant, title: variant })}>
                                            {variant}
                                        </bs.Button>
                                    ))}
                                </div>
                            </div>

                            <CodeBlock>{`context.modal.open(<p>Content</p>, { variant: "warning", title: "warning" });`}</CodeBlock>
                        </div>

                        {/* Modal Example 3: Confirm */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">3</bs.Badge>Confirm dialog</h2>
                            <p>
                                <code>variant: "confirm"</code> shows a question-mark icon plus a split Cancel/Confirm footer, wired to the <code>onConfirm</code>/<code>onCancel</code> callbacks (not a Promise). <code>onClose</code> still fires afterward regardless of which button was clicked.
                            </p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <bs.Button
                                    variant="danger"
                                    onClick={() => context.modal.open("This action cannot be undone. Are you sure?", {
                                        variant: "confirm",
                                        title: "Delete item",
                                        confirmLabel: "Delete",
                                        cancelLabel: "Keep it",
                                        onConfirm: () => context.toast.open("Item deleted", { variant: "success" }),
                                        onCancel: () => context.toast.open("Cancelled", { variant: "normal" }),
                                    })}
                                >
                                    Delete item…
                                </bs.Button>
                            </div>

                            <CodeBlock>{`context.modal.open("This action cannot be undone. Are you sure?", {
    variant: "confirm",
    title: "Delete item",
    confirmLabel: "Delete",
    cancelLabel: "Keep it",
    onConfirm: () => context.toast.open("Item deleted", { variant: "success" }),
    onCancel: () => context.toast.open("Cancelled", { variant: "normal" }),
});`}</CodeBlock>
                        </div>

                        {/* Modal Example 4: Nesting */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">4</bs.Badge>Nesting — opening one from inside another</h2>
                            <p>A modal's own content can open another modal (here, a button inside <code>NestedLevel</code> opens another <code>NestedLevel</code>) — genuinely nested, as deep as you want, each one stacked above the previous.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <bs.Button variant="dark" onClick={() => context.modal.open(<NestedLevel level={1} />, { title: "Level 1" })}>
                                    Open level 1
                                </bs.Button>
                            </div>

                            <CodeBlock>{`function NestedLevel({ level }) {
    return (
        <button onClick={() => context.modal.open(<NestedLevel level={level + 1} />, { title: \`Level \${level + 1}\` })}>
            Open level {level + 1} from here
        </button>
    );
}
context.modal.open(<NestedLevel level={1} />, { title: "Level 1" });`}</CodeBlock>
                        </div>

                        {/* Modal Example 5: Form inside a modal */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">5</bs.Badge>A real form with validation</h2>
                            <p>A full <code>Skeleton.Form</code> with its own <code>Ruleset</code> validation, disabling Save until the form is valid.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <bs.Button variant="outline-primary" onClick={() => context.modal.open(<EditForm onSaved={(title) => context.toast.open(`Saved: ${title}`, { variant: "success" })} />, { title: "Edit item" })}>
                                    Open form modal
                                </bs.Button>
                            </div>

                            <CodeBlock>{`context.modal.open(<EditForm onSaved={handleSave} />, { title: "Edit item" });
// EditForm validates with Ruleset and disables Save until valid.`}</CodeBlock>
                        </div>

                        {/* Modal Example 6: Long content */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">6</bs.Badge>Long content</h2>
                            <p>The dialog grows with its content; the page behind the backdrop scrolls once it's taller than the viewport.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <bs.Button variant="outline-secondary" onClick={() => context.modal.open(<LongContent />, { title: "Long content", size: "lg" })}>
                                    Open long content modal
                                </bs.Button>
                            </div>

                            <CodeBlock>{`context.modal.open(<VeryLongContent />, { title: "Long content", size: "lg" });`}</CodeBlock>
                        </div>

                        {/* Modal Example 7: Stacking with other overlays */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">7</bs.Badge>Stacking with other overlays</h2>
                            <p>Modals share the same z-index pool as offcanvas panels — opening one from inside the other stacks correctly by real open order.</p>

                            <div className="mb-3">
                                <h6 className="text-muted mb-3">RESULT</h6>
                                <bs.Button
                                    variant="dark"
                                    onClick={() => context.offcanvas.open(
                                        <bs.Button variant="warning" onClick={() => context.modal.open(<p>This modal is above the offcanvas.</p>, { title: "On top" })}>
                                            Open a modal from here
                                        </bs.Button>,
                                        { title: "An offcanvas" }
                                    )}
                                >
                                    Open offcanvas, then a modal from it
                                </bs.Button>
                            </div>

                            <CodeBlock>{`context.offcanvas.open(<PanelWithButton />, { title: "An offcanvas" });
// ...then, from inside the panel:
context.modal.open(<p>On top</p>, { title: "On top" });`}</CodeBlock>
                        </div>

                        {/* Modal API Reference */}
                        <div className="mb-5">
                            <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="secondary" className="me-2">API</bs.Badge>Modal — API Reference</h2>

                            <h6 className="mt-4 mb-2"><code>context.modal.open(content, options)</code></h6>
                            <div className="table-responsive mb-4">
                                <table className="table table-bordered">
                                    <thead>
                                        <tr>
                                            <th style={{ width: "20%" }}>Option</th>
                                            <th style={{ width: "20%" }}>Type / default</th>
                                            <th>Description</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        <tr><td><code>content</code></td><td>string | React element</td><td>Text, HTML string, or React element.</td></tr>
                                        <tr><td><code>size</code></td><td>string, default none</td><td><code>sm</code> | <code>lg</code> | <code>xl</code>.</td></tr>
                                        <tr><td><code>variant</code></td><td>string, default <code>"normal"</code></td><td><code>normal</code> | <code>info</code> | <code>success</code> | <code>warning</code> | <code>error</code> | <code>confirm</code></td></tr>
                                        <tr><td><code>title</code></td><td>string</td><td>Bold title shown at the top of the body (no separate header bar).</td></tr>
                                        <tr><td><code>onClose</code></td><td>function</td><td>Called once the modal has fully closed, regardless of how.</td></tr>
                                        <tr><td><code>onConfirm</code> / <code>onCancel</code></td><td>function</td><td>Only used when <code>variant: "confirm"</code>.</td></tr>
                                        <tr><td><code>confirmLabel</code> / <code>cancelLabel</code></td><td>string, default <code>"Confirm"</code> / <code>"Cancel"</code></td><td>Only used when <code>variant: "confirm"</code>.</td></tr>
                                    </tbody>
                                </table>
                            </div>
                            <p className="text-muted"><code>modal.open(...)</code> returns the generated id — keep it to close this exact modal later with <code>context.modal.close(id)</code>. Calling <code>modal.close()</code> with no id closes the topmost one.</p>
                        </div>
                    </div>
                </bs.Tab>
            </bs.Tabs>
        </div>
    );
}
