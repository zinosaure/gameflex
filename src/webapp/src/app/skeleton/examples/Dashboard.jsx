import React from "react";
import { Link } from "react-router-dom";
import * as bs from "react-bootstrap";


const COMPONENT_PAGES = [
	{ to: "/howto/form", icon: "bi bi-ui-checks-grid", title: "Form", description: "Every field component, validation rulesets, and Form.submit." },
	{ to: "/howto/tables", icon: "bi bi-table", title: "Table & Pagination", description: "Tables with inline editing, action panels, and pagination strategies." },
	{ to: "/howto/offcanvas", icon: "bi bi-layout-sidebar-inset", title: "Offcanvas", description: "Slide-in panels — positions, sizes, speed, and stacking multiple levels." },
	{ to: "/howto/feedbacks", icon: "bi bi-bell", title: "Feedbacks", description: "Toasts, alerts and modals — positions, variants, and a built-in confirm flow." },
];

const CodeBlock = ({ children }) => (
	<pre className="bg-body-tertiary border p-3 rounded mb-0"><code style={{ fontSize: "0.875rem" }}>{children}</code></pre>
);

export default function Dashboard() {
	return (
		<div className="container">
			{/* Hero */}
			<div className="mb-5">
				<div className="d-flex flex-wrap gap-2 mb-3">
					<bs.Badge bg="primary">React 18</bs.Badge>
					<bs.Badge bg="secondary">Bootstrap 5.3</bs.Badge>
					<bs.Badge bg="success">FastAPI</bs.Badge>
					<bs.Badge bg="dark">Docker Compose</bs.Badge>
				</div>
				<h1 className="display-4 fw-bold mb-3">Skeleton Template</h1>
				<p className="lead text-muted mb-4">
					A clone-and-go starter kit for quickly bootstrapping a new web app: a React + Bootstrap frontend talking to a backend over HTTP,
					both orchestrated by Docker Compose. Ships with a working example domain so you can see every piece wired together —
					strip it out once you understand the pattern.
				</p>
				<div className="d-flex flex-wrap gap-2">
					<bs.Button as="a" href="#quickstart" variant="primary">Quickstart <i className="bi bi-arrow-down"></i></bs.Button>
					<bs.Button as="a" href="#components" variant="outline-secondary">Browse components</bs.Button>
				</div>
			</div>

			{/* Quickstart */}
			<div id="quickstart" className="mb-5">
				<h2 className="border-bottom pb-2 mb-3"><i className="bi bi-rocket-takeoff me-2 text-primary"></i>Quickstart</h2>
				<bs.Row className="g-4">
					<bs.Col md={7}>
						<p>Launch every service with the provided script — it wraps <code>docker compose</code> and asks whether to rebuild images:</p>
						<CodeBlock>{`./launch.sh`}</CodeBlock>
						<p className="text-muted small mt-2 mb-0">First run downloads and builds the images, so it takes a bit longer.</p>
					</bs.Col>
					<bs.Col md={5}>
						<div className="table-responsive">
							<table className="table table-bordered mb-0">
								<thead>
									<tr><th>Service</th><th>URL</th></tr>
								</thead>
								<tbody>
									<tr><td>Frontend (this app)</td><td><code>localhost:10103</code></td></tr>
									<tr><td>Backend API</td><td><code>localhost:10102</code></td></tr>
								</tbody>
							</table>
						</div>
					</bs.Col>
				</bs.Row>
			</div>

			{/* Architecture */}
			<div className="mb-5">
				<h2 className="border-bottom pb-2 mb-3"><i className="bi bi-diagram-3 me-2 text-primary"></i>Architecture</h2>
				<p>Each service is an independent Docker container; <code>docker-compose.yaml</code> wires them together on a shared network.</p>
				<bs.Row className="g-4">
					<bs.Col md={6}>
						<CodeBlock>{`docker-compose.yaml
├── nodejs/    ← frontend (React), always here
└── python/    ← backend (FastAPI), reference impl.`}</CodeBlock>
					</bs.Col>
					<bs.Col md={6}>
						<bs.Alert variant="info" className="mb-0">
							<i className="bi bi-info-circle me-2"></i>
							<strong>The backend is not tied to Python.</strong> It's just a container exposing an HTTP API. Add a sibling
							folder (<code>go/</code>, <code>rust/</code>...) with its own <code>Dockerfile</code>, a matching service in
							<code> docker-compose.yaml</code>, and point <code>BACKEND_PROXY_TARGET</code> at it — that env var
							(read by <code>devServer.proxy</code> in <code>nodejs/src/webpack.config.js</code>) is the <em>only</em> place the frontend knows about the backend.
						</bs.Alert>
					</bs.Col>
				</bs.Row>
			</div>

			{/* Backend API */}
			<div className="mb-5">
				<h2 className="border-bottom pb-2 mb-3"><i className="bi bi-server me-2 text-primary"></i>How the backend API works</h2>
				<p>
					Models inherit <code>DeclarativeBase</code> (<code>python/src/app/libs/skeleton.py</code>) and register REST endpoints with one call:
				</p>
				<CodeBlock>{`Game.REST(
    api=app,
    prefix="/api/games",
    tags=["games"],
    scopes=["LIST", "FETCH", "POST", "PATCH", "DELETE"],
)`}</CodeBlock>

				<bs.Accordion className="mt-3" alwaysOpen={false}>
					<bs.Accordion.Item eventKey="0">
						<bs.Accordion.Header>Generated endpoints & response envelope</bs.Accordion.Header>
						<bs.Accordion.Body>
							<div className="table-responsive mb-3">
								<table className="table table-sm table-bordered mb-0">
									<thead><tr><th>Method</th><th>URL</th><th>Scope</th></tr></thead>
									<tbody>
										<tr><td>GET</td><td><code>{'{prefix}'}</code></td><td>LIST (paginated, filterable, searchable)</td></tr>
										<tr><td>GET</td><td><code>{'{prefix}/{id}'}</code></td><td>FETCH</td></tr>
										<tr><td>POST</td><td><code>{'{prefix}'}</code></td><td>POST — JSON or multipart (file uploads)</td></tr>
										<tr><td>PATCH</td><td><code>{'{prefix}/{id}'}</code></td><td>PATCH — partial update</td></tr>
										<tr><td>DELETE</td><td><code>{'{prefix}/{id}'}</code></td><td>DELETE</td></tr>
									</tbody>
								</table>
							</div>
							<p className="mb-2">Every <code>/api/*</code> response is wrapped in the same envelope, success or failure:</p>
							<CodeBlock>{`{ "at": "...", "code": 200, "state": "success", "reference": "item", "item": {...} }
{ "at": "...", "code": 200, "state": "success", "reference": "items", "items": [...], "pagination": {...} }
{ "at": "...", "code": 400, "state": "failed", "reference": "exception", "exception": { "name": "...", "description": "...", "detail": "..." } }`}</CodeBlock>
						</bs.Accordion.Body>
					</bs.Accordion.Item>
					<bs.Accordion.Item eventKey="1">
						<bs.Accordion.Header>Search & filtering DSL</bs.Accordion.Header>
						<bs.Accordion.Body>
							<p>The <code>search</code> query param takes a JSON object (AND) or an array of objects (OR between groups):</p>
							<CodeBlock>{`GET /api/games?search=[{"genre":{"intersect":["RPG"]}},{"publisher":"Nintendo"}]
// → genre intersects "RPG"  OR  publisher = "Nintendo"`}</CodeBlock>
							<p className="mt-2 mb-0 text-muted small">
								Operators: <code>{'>'} {'<'} {'>='} {'<='} !=</code>, <code>between</code>/<code>not_between</code>, <code>in</code>/<code>not_in</code>,{" "}
								<code>like</code>/<code>startwith</code>/<code>endwith</code> (+ <code>not_*</code>), <code>null</code>/<code>not_null</code>,{" "}
								<code>intersect</code>/<code>not_intersect</code> (JSON array columns). Only fields listed in a model's <code>__searchable_fields__</code> are queryable.
							</p>
						</bs.Accordion.Body>
					</bs.Accordion.Item>
					<bs.Accordion.Item eventKey="2">
						<bs.Accordion.Header>File uploads</bs.Accordion.Header>
						<bs.Accordion.Body className="mb-0">
							Post <code>multipart/form-data</code> to the same POST/PATCH endpoints — no separate upload endpoint. Override{" "}
							<code>upload_callback(cls, name, content, tmp_file)</code> on your model to control per-field size limits and storage location.
						</bs.Accordion.Body>
					</bs.Accordion.Item>
				</bs.Accordion>
			</div>

			{/* Frontend framework */}
			<div className="mb-5">
				<h2 className="border-bottom pb-2 mb-3"><i className="bi bi-window-stack me-2 text-primary"></i>The frontend framework</h2>
				<p>
					<code>nodejs/src/app/skeleton/skeleton.js</code> exposes a React context with everything needed for dialogs/notifications —
					native Bootstrap components under the hood, portal-rendered so they keep full access to your app's context and router.
				</p>
				<div className="table-responsive mb-3">
					<table className="table table-bordered">
						<thead><tr><th style={{ width: "30%" }}>API</th><th>What it does</th></tr></thead>
						<tbody>
							<tr><td><Link to="/howto/offcanvas"><code>context.offcanvas.open/close</code></Link></td><td>Slide-in panels, stackable, any of the 4 edges.</td></tr>
							<tr><td><Link to="/howto/feedbacks"><code>context.toast.open/close</code></Link></td><td>Corner notifications, 9 positions.</td></tr>
							<tr><td><Link to="/howto/feedbacks"><code>context.modal.open/close</code></Link></td><td>Centered dialogs, with a built-in confirm flow.</td></tr>
							<tr><td><Link to="/howto/feedbacks"><code>context.alert()</code></Link></td><td>Dismissible banner pinned under the header.</td></tr>
						</tbody>
					</table>
				</div>
				<p className="mb-2">Routes are declared in <code>nodejs/src/app/index.js</code> using a dotted-path convention:</p>
				<CodeBlock>{`{ path: { to: "/howto/form", component: "skeleton.examples.Form" } }
// → resolves to nodejs/src/app/skeleton/examples/Form.jsx`}</CodeBlock>

				<bs.Alert variant="secondary" className="mt-3 mb-0">
					<i className="bi bi-css me-2"></i>
					<strong>No custom CSS.</strong> This template never styles what Bootstrap already provides — everything above is
					plain Bootstrap classes and JS components. The only stylesheet (<code>skeleton-themes.css</code>) exists solely for
					the handful of things Bootstrap has zero native equivalent for.
				</bs.Alert>
			</div>

			{/* Adopting the template */}
			<div className="mb-5">
				<h2 className="border-bottom pb-2 mb-3"><i className="bi bi-hammer me-2 text-primary"></i>Starting a real project from this template</h2>
				<bs.Row className="g-4">
					<bs.Col md={6}>
						<h5>Backend</h5>
						<ol className="ps-3">
							<li>Set <code>SKELETON_EXAMPLE_MODE = False</code> in <code>python/src/app/main.py</code>.</li>
							<li>Define your own models (inherit <code>DeclarativeBase</code>), following the <code>Console</code>/<code>Game</code> pattern in <code>python/src/app/example/retrogaming.py</code>.</li>
							<li>Register them: <code>MyModel.REST(api=app, prefix="/api/...", scopes=[...])</code>.</li>
							<li>Delete <code>python/src/app/example/</code> once you no longer need it as reference.</li>
						</ol>
					</bs.Col>
					<bs.Col md={6}>
						<h5>Frontend</h5>
						<ol className="ps-3">
							<li>Replace <code>menuItems</code>/<code>homePage</code> in <code>nodejs/src/app/index.js</code> with your own pages.</li>
							<li>Reuse <code>Skeleton.Form</code>/<code>Skeleton.Table</code>/<code>usePagination</code> for CRUD screens.</li>
							<li>Use the context API above for dialogs/notifications instead of rolling your own.</li>
							<li>Keep following the no-custom-CSS rule.</li>
						</ol>
					</bs.Col>
				</bs.Row>
			</div>

			{/* Components */}
			<div id="components" className="mb-5">
				<h2 className="border-bottom pb-2 mb-3"><i className="bi bi-grid-1x2 me-2 text-primary"></i>Explore the components</h2>
				<bs.Row xs={1} md={2} lg={3} className="g-4">
					{COMPONENT_PAGES.map(page => (
						<bs.Col key={page.to} md={6} lg={6}>
							<Link to={page.to} className="text-decoration-none text-reset">
								<bs.Card className="h-100 shadow-sm">
									<bs.Card.Body>
										<div className="d-flex align-items-center gap-2 mb-2">
											<i className={`${page.icon} fs-4 text-primary`}></i>
											<bs.Card.Title className="mb-0">{page.title}</bs.Card.Title>
										</div>
										<bs.Card.Text className="text-muted">{page.description}</bs.Card.Text>
									</bs.Card.Body>
									<bs.Card.Footer className="bg-transparent border-0 pt-0">
										<span className="text-primary small fw-semibold">
											View docs <i className="bi bi-arrow-right"></i>
										</span>
									</bs.Card.Footer>
								</bs.Card>
							</Link>
						</bs.Col>
					))}
				</bs.Row>
			</div>

			<p className="text-muted small border-top pt-3 mb-5">
				Looking for deeper architecture notes? See <code>AGENTS.md</code> and <code>CODE_CONVENTIONS.md</code> at the repo root, and <code>docs/</code> for the
				AI-agent-facing reference docs.
			</p>
		</div>
	);
}
