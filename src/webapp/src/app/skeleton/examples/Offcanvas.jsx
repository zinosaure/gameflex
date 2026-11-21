import React, { useContext } from 'react';
import * as bs from 'react-bootstrap';
import Skeleton from "../skeleton.js";


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

export default function OffcanvasDoc() {
    const context = useContext(Skeleton.context);

    const Menu = ({ title }) => (
        <div>
            <p className="text-muted">Content rendered inside the offcanvas ({title}).</p>
            <bs.Button size="sm" variant="outline-secondary" onClick={() => context.offcanvas.close()}>
                Close this panel
            </bs.Button>
        </div>
    );

    // A panel that opens ANOTHER instance of itself — true nesting, triggered
    // from a button that lives inside the already-open panel's own content.
    const NestedLevel = ({ level = 1 }) => (
        <div>
            <p>You are at nesting level <bs.Badge bg="dark">{level}</bs.Badge>. Each level is opened from a button inside the previous one's content.</p>
            <div className="d-flex gap-2">
                <bs.Button
                    size="sm"
                    variant="primary"
                    onClick={() => context.offcanvas.open(<NestedLevel level={level + 1} />, { position: level % 2 ? "end" : "start", size: 0.9, title: `Level ${level + 1}` })}
                >
                    Open level {level + 1} from here <i className="bi bi-arrow-right"></i>
                </bs.Button>
                <bs.Button size="sm" variant="outline-secondary" onClick={() => context.offcanvas.close()}>Close this level</bs.Button>
            </div>
        </div>
    );

    const FormPanel = () => {
        const [values, setValues] = React.useState({});
        const context_ = { values, setValues, files: {}, setFiles: () => { }, rulesets: {} };

        return (
            <div>
                <p className="text-muted">A real <code>Skeleton.Form</code> rendered inside an offcanvas panel — full context/hooks access via <code>createPortal</code>.</p>
                <Skeleton.Form>
                    <Skeleton.Form.Group label="Name"><Skeleton.Form.Text name="name" context={context_} /></Skeleton.Form.Group>
                    <Skeleton.Form.Group label="Notes"><Skeleton.Form.Longtext name="notes" context={context_} props={{ rows: 3 }} /></Skeleton.Form.Group>
                    <bs.Button
                        variant="primary"
                        onClick={() => {
                            context.offcanvas.close();
                            context.toast.open(`Saved: ${values.name || "(no name)"}`, { variant: "success" });
                        }}
                    >
                        Save
                    </bs.Button>
                </Skeleton.Form>
            </div>
        );
    };

    const LongContent = () => (
        <div>
            <div className="d-flex justify-content-end position-sticky top-0 pb-2 mb-2 border-bottom" style={{ backgroundColor: 'var(--bs-body-bg)', zIndex: 1 }}>
                <bs.Button size="sm" variant="outline-secondary" onClick={() => context.offcanvas.close()}>
                    Close
                </bs.Button>
            </div>
            {Array.from({ length: 30 }, (_, i) => (
                <p key={i}>Line {i + 1} — this panel has no size set, so it covers the full page; the scrollbar spans the whole viewport height, not just the body area.</p>
            ))}
        </div>
    );

    return (
        <div className="container">
            <div className="mb-5">
                <h1 className="display-5 fw-bold mb-3">Offcanvas</h1>
                <p className="lead text-muted">
                    Slide-in panels built on the native Bootstrap Offcanvas component — positions, sizes, transition speed, real nesting, and any content type.
                </p>
            </div>

            {/* Example 1: Positions */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">1</bs.Badge>Positions</h2>
                <p>Four positions are supported: <code>start</code>, <code>end</code> (default), <code>top</code>, <code>bottom</code>.</p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <div className="d-flex gap-2 flex-wrap">
                        {["start", "end", "top", "bottom"].map(position => (
                            <bs.Button key={position} variant="outline-primary" onClick={() => context.offcanvas.open(<Menu label={position} />, { position, title: `Position: ${position}` })}>
                                {position}
                            </bs.Button>
                        ))}
                    </div>
                </div>

                <CodeBlock>{`context.offcanvas.open(<Menu />, {
    position: "start", // "start" | "end" | "top" | "bottom"
    title: "Position: start",
});`}</CodeBlock>
            </div>

            {/* Example 2: Sizes */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">2</bs.Badge>Sizes</h2>
                <p>
                    <code>size</code> accepts a pixel value, or a fraction between 0 and 1 (percentage of the viewport). The default (no <code>size</code>) covers the whole page.
                </p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <div className="d-flex gap-2 flex-wrap">
                        <bs.Button variant="outline-primary" onClick={() => context.offcanvas.open(<Menu label="450px" />, { size: 450, title: "size: 450" })}>
                            450px
                        </bs.Button>
                        <bs.Button variant="outline-primary" onClick={() => context.offcanvas.open(<Menu label="40%" />, { size: 0.4, title: "size: 0.4" })}>
                            40% of viewport
                        </bs.Button>
                        <bs.Button variant="outline-primary" onClick={() => context.offcanvas.open(<Menu label="full page" />, { title: "size: default" })}>
                            Full page (default)
                        </bs.Button>
                    </div>
                </div>

                <CodeBlock>{`context.offcanvas.open(<Menu />, { size: 450 });   // 450px
context.offcanvas.open(<Menu />, { size: 0.4 });   // 40% of the viewport
context.offcanvas.open(<Menu />, {});              // full page (default)`}</CodeBlock>
            </div>

            {/* Example 3: Speed */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">3</bs.Badge>Transition speed</h2>
                <p><code>speed</code> is the transition duration in milliseconds (default <code>500</code>).</p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <div className="d-flex gap-2 flex-wrap">
                        <bs.Button variant="outline-primary" onClick={() => context.offcanvas.open(<Menu label="150ms" />, { speed: 150, title: "speed: 150" })}>
                            Fast (150ms)
                        </bs.Button>
                        <bs.Button variant="outline-primary" onClick={() => context.offcanvas.open(<Menu label="1200ms" />, { speed: 1200, title: "speed: 1200" })}>
                            Slow (1200ms)
                        </bs.Button>
                    </div>
                </div>

                <CodeBlock>{`context.offcanvas.open(<Menu />, { speed: 150 });  // fast
context.offcanvas.open(<Menu />, { speed: 1200 }); // slow`}</CodeBlock>
            </div>

            {/* Example 4: Stacking several at once */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">4</bs.Badge>Stacking several at once</h2>
                <p>
                    Each call to <code>offcanvas.open</code> stacks a new panel above the previous one, with an increasing z-index. Only the topmost backdrop stays visible; <code>offcanvas.close()</code> always closes the topmost panel first.
                </p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <bs.Button
                        variant="dark"
                        onClick={() => {
                            context.offcanvas.open(<Menu label="level 1" />, { position: "start", title: "Level 1" });
                            context.offcanvas.open(<Menu label="level 2" />, { position: "end", size: 0.5, title: "Level 2" });
                            context.offcanvas.open(<Menu label="level 3" />, { position: "bottom", size: 0.3, title: "Level 3" });
                        }}
                    >
                        Open 3 stacked levels at once
                    </bs.Button>
                </div>

                <CodeBlock>{`context.offcanvas.open(<Level1 />, { position: "start" });
context.offcanvas.open(<Level2 />, { position: "end", size: 0.5 });
context.offcanvas.open(<Level3 />, { position: "bottom", size: 0.3 });
// offcanvas.close() closes Level3 first, then Level2, then Level1.`}</CodeBlock>
            </div>

            {/* Example 5: True nesting — opened from inside another */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">5</bs.Badge>Nesting — opening one from inside another</h2>
                <p>
                    The more common real case: a panel's own content calls <code>offcanvas.open</code> again (here, a button inside <code>NestedLevel</code> opens
                    another <code>NestedLevel</code>) — not several calls fired at once, but a genuine offcanvas-inside-an-offcanvas interaction, as deep as you want.
                </p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <bs.Button variant="dark" onClick={() => context.offcanvas.open(<NestedLevel level={1} />, { position: "end", size: 0.9, title: "Level 1" })}>
                        Open level 1
                    </bs.Button>
                </div>

                <CodeBlock>{`function NestedLevel({ level }) {
    return (
        <button onClick={() => context.offcanvas.open(
            <NestedLevel level={level + 1} />,
            { title: \`Level \${level + 1}\` }
        )}>
            Open level {level + 1} from here
        </button>
    );
}
context.offcanvas.open(<NestedLevel level={1} />, { title: "Level 1" });`}</CodeBlock>
            </div>

            {/* Example 6: Form inside a panel */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">6</bs.Badge>A real form inside a panel</h2>
                <p>Any React content works, including a full <code>Skeleton.Form</code> with its own local state.</p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <bs.Button variant="outline-primary" onClick={() => context.offcanvas.open(<FormPanel />, { position: "end", size: 420, title: "Edit item" })}>
                        Open form panel
                    </bs.Button>
                </div>

                <CodeBlock>{`context.offcanvas.open(<FormPanel />, { size: 420, title: "Edit item" });
// FormPanel keeps its own local values/context and calls
// context.offcanvas.close() + context.toast.open(...) on save.`}</CodeBlock>
            </div>

            {/* Example 7: Long scrollable content */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">7</bs.Badge>Long, scrollable content</h2>
                <p>The panel's scrollbar spans the full page height (not just below the header) even for content much taller than the viewport.</p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <bs.Button variant="outline-secondary" onClick={() => context.offcanvas.open(<LongContent />, { title: "Long content" })}>
                        Open long content panel
                    </bs.Button>
                </div>

                <CodeBlock>{`context.offcanvas.open(<VeryLongContent />, { title: "Long content" });`}</CodeBlock>
            </div>

            {/* Example 8: onClose callback */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">8</bs.Badge>onClose callback</h2>
                <p><code>onClose</code> fires once the panel has fully finished closing, however it was dismissed (X button, backdrop click, ESC, or <code>offcanvas.close()</code>).</p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <bs.Button
                        variant="outline-secondary"
                        onClick={() => context.offcanvas.open(<Menu label="onClose demo" />, {
                            title: "Watch the console",
                            onClose: () => context.toast.open("Offcanvas closed", { variant: "info" }),
                        })}
                    >
                        Open (toast on close)
                    </bs.Button>
                </div>

                <CodeBlock>{`context.offcanvas.open(<Menu />, {
    onClose: () => context.toast.open("Offcanvas closed", { variant: "info" }),
});`}</CodeBlock>
            </div>

            {/* API Reference */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="secondary" className="me-2">API</bs.Badge>API Reference</h2>

                <h6 className="mt-4 mb-2"><code>context.offcanvas.open(component, options)</code></h6>
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
                            <tr><td><code>component</code></td><td>React element</td><td>Content rendered inside the panel body.</td></tr>
                            <tr><td><code>position</code></td><td>string, default <code>"end"</code></td><td><code>start</code> | <code>end</code> | <code>top</code> | <code>bottom</code></td></tr>
                            <tr><td><code>size</code></td><td>number, default full page</td><td>Pixels, or a fraction &lt;1 of the viewport (width for start/end, height for top/bottom).</td></tr>
                            <tr><td><code>speed</code></td><td>number (ms), default <code>500</code></td><td>Transition duration.</td></tr>
                            <tr><td><code>title</code></td><td>string</td><td>Offcanvas header title.</td></tr>
                            <tr><td><code>onClose</code></td><td>function</td><td>Called once the panel has fully closed.</td></tr>
                        </tbody>
                    </table>
                </div>

                <h6 className="mt-4 mb-2"><code>context.offcanvas.close()</code></h6>
                <p className="text-muted">Closes the topmost open panel (LIFO). Takes no arguments — call it once per level to unwind a nested stack.</p>
            </div>
        </div>
    );
}
