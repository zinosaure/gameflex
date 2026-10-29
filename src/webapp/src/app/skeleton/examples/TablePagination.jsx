import React, { useState, useContext } from 'react';
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

export default function TablePaginationDoc() {
    const context = useContext(Skeleton.context);

    // Example 1: Basic Table
    const basicItems = [
        { id: 1, name: "John Doe", email: "john@example.com", role: "Admin" },
        { id: 2, name: "Jane Smith", email: "jane@example.com", role: "User" },
        { id: 3, name: "Bob Johnson", email: "bob@example.com", role: "Manager" },
    ];

    // Example 2: Table with Checkboxes
    const [selectedItems, setSelectedItems] = useState([]);
    const checkboxItems = [
        { id: 1, product: "Laptop", price: 999, stock: 15 },
        { id: 2, product: "Mouse", price: 25, stock: 50 },
        { id: 3, product: "Keyboard", price: 75, stock: 30 },
        { id: 4, product: "Monitor", price: 350, stock: 8 },
    ];

    // Example 3: Table with Inline Editing
    const [editableItems, setEditableItems] = useState([
        { id: 1, title: "Task 1", status: "pending", priority: "high" },
        { id: 2, title: "Task 2", status: "completed", priority: "low" },
        { id: 3, title: "Task 3", status: "in-progress", priority: "medium" },
    ]);

    const inlineEditor = (item, dismissInline) => {
        if (!item) {
            // Add new item
            return (
                <div>
                    <h6 className="mb-3">Create New Task</h6>
                    <bs.Form onSubmit={(e) => {
                        e.preventDefault();
                        const formData = new FormData(e.target);
                        const newItem = {
                            id: Math.max(...editableItems.map(i => i.id)) + 1,
                            title: formData.get('title'),
                            status: formData.get('status'),
                            priority: formData.get('priority'),
                        };
                        setEditableItems([...editableItems, newItem]);
                        context.toast.open("Task created successfully", { variant: "success", title: "Created" });
                        dismissInline();
                    }}>
                        <bs.Row className="g-2">
                            <bs.Col md={4}>
                                <bs.Form.Control name="title" placeholder="Title" required />
                            </bs.Col>
                            <bs.Col md={3}>
                                <bs.Form.Select name="status" required>
                                    <option value="pending">Pending</option>
                                    <option value="in-progress">In Progress</option>
                                    <option value="completed">Completed</option>
                                </bs.Form.Select>
                            </bs.Col>
                            <bs.Col md={3}>
                                <bs.Form.Select name="priority" required>
                                    <option value="low">Low</option>
                                    <option value="medium">Medium</option>
                                    <option value="high">High</option>
                                </bs.Form.Select>
                            </bs.Col>
                            <bs.Col md={2}>
                                <bs.Button type="submit" className="w-100" variant="success">
                                    <i className="bi bi-check"></i> Create
                                </bs.Button>
                            </bs.Col>
                        </bs.Row>
                    </bs.Form>
                </div>
            );
        }

        // Edit existing item
        return (
            <div>
                <h6 className="mb-3">Edit Task: {item.title}</h6>
                <bs.Form onSubmit={(e) => {
                    e.preventDefault();
                    const formData = new FormData(e.target);
                    const updatedItem = {
                        ...item,
                        title: formData.get('title'),
                        status: formData.get('status'),
                        priority: formData.get('priority'),
                    };
                    setEditableItems(editableItems.map(i => i.id === item.id ? updatedItem : i));
                    context.toast.open("Task updated successfully", { variant: "success", title: "Updated" });
                    dismissInline();
                }}>
                    <bs.Row className="g-2">
                        <bs.Col md={4}>
                            <bs.Form.Control name="title" defaultValue={item.title} required />
                        </bs.Col>
                        <bs.Col md={3}>
                            <bs.Form.Select name="status" defaultValue={item.status} required>
                                <option value="pending">Pending</option>
                                <option value="in-progress">In Progress</option>
                                <option value="completed">Completed</option>
                            </bs.Form.Select>
                        </bs.Col>
                        <bs.Col md={3}>
                            <bs.Form.Select name="priority" defaultValue={item.priority} required>
                                <option value="low">Low</option>
                                <option value="medium">Medium</option>
                                <option value="high">High</option>
                            </bs.Form.Select>
                        </bs.Col>
                        <bs.Col md={2}>
                            <bs.Button type="submit" className="w-100" variant="primary">
                                <i className="bi bi-check"></i> Save
                            </bs.Button>
                        </bs.Col>
                    </bs.Row>
                </bs.Form>
            </div>
        );
    };

    // Example 4: Table with Action Panels
    const [actionItems, setActionItems] = useState([
        { id: 1, order: "#1001", customer: "John Doe", total: 150, status: "shipped" },
        { id: 2, order: "#1002", customer: "Jane Smith", total: 250, status: "pending" },
        { id: 3, order: "#1003", customer: "Bob Johnson", total: 99, status: "delivered" },
    ]);
    const [actionSelected, setActionSelected] = useState([]);

    // Example 5: Standard Pagination
    const allPaginatedItems = Array.from({ length: 53 }, (_, i) => ({
        id: i + 1,
        product: `Product ${i + 1}`,
        price: Math.floor(Math.random() * 1000) + 10,
        category: ['Electronics', 'Clothing', 'Food', 'Books'][Math.floor(Math.random() * 4)]
    }));

    const [isPending5, setIsPending5] = useState(false);
    const [currentPage5, setCurrentPage5] = useState(1);
    const itemsPerPage5 = 5;

    const pagination5 = Skeleton.usePagination({
        isPending: isPending5,
        currentPosition: currentPage5,
        itemCount: allPaginatedItems.length,
        itemsPerPage: itemsPerPage5,
        onPositionChange: (newPage) => {
            setIsPending5(true);
            setCurrentPage5(newPage);
            setTimeout(() => setIsPending5(false), 300);
        },
        maxButtons: 7
    });

    const visibleItems5 = allPaginatedItems.slice(
        (currentPage5 - 1) * itemsPerPage5,
        currentPage5 * itemsPerPage5
    );

    // Example 6: Token-Based Pagination
    const [isPending6, setIsPending6] = useState(false);
    const [tokenItems, setTokenItems] = useState([]);
    const [totalCount6, setTotalCount6] = useState(0);
    const [currentToken, setCurrentToken] = useState(null);

    const [tokenPagination, setPaginationToken, resetPagination] = Skeleton.usePagination.TokenBased({
        isPending: isPending6,
        onTokenChange: (token) => {
            setIsPending6(true);
            setCurrentToken(token);

            // Simulate API call
            setTimeout(() => {
                const mockData = generateMockPage(token);
                setTokenItems(mockData.items);
                const total = setPaginationToken(token, mockData.nextToken, 5, mockData.items.length);
                setTotalCount6(total); // Update total count based on items loaded
                setIsPending6(false);
            }, 500);
        },
        limitSwitch: true,
        maxButtons: 7
    });

    // Initialize token pagination on mount
    React.useEffect(() => {
        if (tokenItems.length === 0) {
            setIsPending6(true);
            setTimeout(() => {
                const mockData = generateMockPage(null);
                setTokenItems(mockData.items);
                const total = setPaginationToken(null, mockData.nextToken, 5, mockData.items.length);
                setTotalCount6(total); // Update total count based on items loaded
                setIsPending6(false);
            }, 500);
        }
    }, []);

    function generateMockPage(token) {
        const pageNum = token ? parseInt(token.split('-')[1]) : 1;
        const hasNext = pageNum < 5; // Only 5 pages total
        
        return {
            items: Array.from({ length: 5 }, (_, i) => ({
                id: `${pageNum}-${i + 1}`,
                name: `Item ${(pageNum - 1) * 5 + i + 1}`,
                value: Math.floor(Math.random() * 100)
            })),
            nextToken: hasNext ? `page-${pageNum + 1}` : null
        };
    }

    // Example 7: Skeleton.usePagination.DataTable
    const allDataTableItems = Array.from({ length: 30 }, (_, i) => ({
        id: i + 1,
        user: `User ${i + 1}`,
        email: `user${i + 1}@example.com`,
        joined: `2024-${String(Math.floor(Math.random() * 12) + 1).padStart(2, '0')}-${String(Math.floor(Math.random() * 28) + 1).padStart(2, '0')}`
    }));

    const [isPending7, setIsPending7] = useState(false);
    const [currentPage7, setCurrentPage7] = useState(1);
    const itemsPerPage7 = 6;

    const [pagination7, paginatedItems7] = Skeleton.usePagination.DataTable({
        isPending: isPending7,
        items: allDataTableItems,
        currentPosition: currentPage7,
        itemsPerPage: itemsPerPage7,
        onPositionChange: (newPage) => {
            setIsPending7(true);
            setCurrentPage7(newPage);
            setTimeout(() => setIsPending7(false), 300);
        },
        maxButtons: 9
    });

    return (
        <div className="container">
            <div className="mb-5">
                <h1 className="display-5 fw-bold mb-3">Table & Pagination</h1>
                <p className="lead text-muted">
                    Data tables with inline editing, checkboxes, custom actions, and flexible pagination strategies.
                </p>
            </div>

            {/* Example 1: Basic Table */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">1</bs.Badge>Basic Table</h2>
                <p>
                    Create a simple data table with custom labels and callbacks for dynamic content.
                </p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <Skeleton.Table
                        labels={{
                            id: {
                                title: "ID",
                                css: { width: '60px', fontWeight: 'bold' },
                                callback: "id"
                            },
                            name: {
                                title: "Name",
                                callback: "name"
                            },
                            email: {
                                title: "Email",
                                callback: "email"
                            },
                            role: {
                                title: "Role",
                                css: { width: '120px' },
                                callback: (item) => (
                                    <bs.Badge bg={
                                        item.role === 'Admin' ? 'danger' :
                                        item.role === 'Manager' ? 'warning' : 'secondary'
                                    }>
                                        {item.role}
                                    </bs.Badge>
                                )
                            }
                        }}
                        items={basicItems}
                        itemCount={basicItems.length}
                        itemsPerPage={10}
                    />
                </div>

                <CodeBlock>{`import React from 'react';
import Skeleton from "../skeleton.js";
import { Badge } from 'react-bootstrap';

function BasicTable() {
    const items = [
        { id: 1, name: "John Doe", email: "john@example.com", role: "Admin" },
        { id: 2, name: "Jane Smith", email: "jane@example.com", role: "User" },
        { id: 3, name: "Bob Johnson", email: "bob@example.com", role: "Manager" }
    ];

    return (
        <Skeleton.Table
            labels={{
                id: {
                    title: "ID",
                    css: { width: '60px', fontWeight: 'bold' },
                    callback: "id"
                },
                name: {
                    title: "Name",
                    callback: "name"
                },
                email: {
                    title: "Email",
                    callback: "email"
                },
                role: {
                    title: "Role",
                    css: { width: '120px' },
                    callback: (item) => (
                        <Badge bg={
                            item.role === 'Admin' ? 'danger' :
                            item.role === 'Manager' ? 'warning' : 'secondary'
                        }>
                            {item.role}
                        </Badge>
                    )
                }
            }}
            items={items}
            itemCount={items.length}
            itemsPerPage={10}
        />
    );
}

export default BasicTable;`}</CodeBlock>
            </div>

            {/* Example 2: Table with Checkboxes */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">2</bs.Badge>Table with Checkboxes</h2>
                <p>
                    Use <code>Skeleton.Table.label.checkbox()</code> to add checkboxes for selecting items. 
                    The checkbox header selects/deselects all items on the current page.
                </p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <div className="mb-3">
                        <bs.Alert variant="info" className="mb-2 py-2">
                            <small>Selected: {selectedItems.length} item(s)</small>
                        </bs.Alert>
                    </div>
                    <Skeleton.Table
                        labels={{
                            checkbox: Skeleton.Table.label.checkbox(checkboxItems, selectedItems, setSelectedItems),
                            product: {
                                title: "Product",
                                callback: "product"
                            },
                            price: {
                                title: "Price",
                                callback: (item) => `$${item.price}`
                            },
                            stock: {
                                title: "Stock",
                                callback: (item) => (
                                    <bs.Badge bg={item.stock > 20 ? 'success' : item.stock > 10 ? 'warning' : 'danger'}>
                                        {item.stock} units
                                    </bs.Badge>
                                )
                            }
                        }}
                        items={checkboxItems}
                        itemCount={checkboxItems.length}
                        itemsPerPage={10}
                        selectedItems={selectedItems}
                    />
                </div>

                <CodeBlock>{`import React, { useState } from 'react';
import Skeleton from "../skeleton.js";
import { Badge, Alert } from 'react-bootstrap';

function CheckboxTable() {
    const [selectedItems, setSelectedItems] = useState([]);

    const items = [
        { id: 1, product: "Laptop", price: 999, stock: 15 },
        { id: 2, product: "Mouse", price: 25, stock: 50 },
        { id: 3, product: "Keyboard", price: 75, stock: 30 },
        { id: 4, product: "Monitor", price: 350, stock: 8 }
    ];

    return (
        <>
            <Alert variant="info" className="mb-2 py-2">
                <small>Selected: {selectedItems.length} item(s)</small>
            </Alert>

            <Skeleton.Table
                labels={{
                    checkbox: Skeleton.Table.label.checkbox(
                        items,              // Current items
                        selectedItems,      // Selected IDs
                        setSelectedItems    // Setter function
                    ),
                    product: {
                        title: "Product",
                        callback: "product"
                    },
                    price: {
                        title: "Price",
                        callback: (item) => \`$\${item.price}\`
                    },
                    stock: {
                        title: "Stock",
                        callback: (item) => (
                            <Badge bg={item.stock > 20 ? 'success' : item.stock > 10 ? 'warning' : 'danger'}>
                                {item.stock} units
                            </Badge>
                        )
                    }
                }}
                items={items}
                itemCount={items.length}
                itemsPerPage={10}
                selectedItems={selectedItems}
            />
        </>
    );
}

export default CheckboxTable;`}</CodeBlock>
            </div>

            {/* Example 3: Inline Editing */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">3</bs.Badge>Inline Editing</h2>
                <p>
                    Enable inline editing with <code>inlinePlus</code> prop. Use <code>Skeleton.Table.label.update()</code> 
                    and <code>Skeleton.Table.label.delete()</code> for edit/delete actions. Click the + icon to add new items.
                </p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <Skeleton.Table
                        labels={{
                            edit: Skeleton.Table.label.update(inlineEditor),
                            delete: Skeleton.Table.label.delete((item, dismissInline) => {
                                return (
                                    <div className="text-center">
                                        <p className="mb-3">Delete task: <strong>{item.title}</strong>?</p>
                                        <bs.Button
                                            variant="danger"
                                            size="sm"
                                            className="me-2"
                                            onClick={() => {
                                                setEditableItems(editableItems.filter(i => i.id !== item.id));
                                                context.toast.open("Task deleted", { variant: "success", title: "Deleted" });
                                                dismissInline();
                                            }}
                                        >
                                            <i className="bi bi-trash"></i> Confirm Delete
                                        </bs.Button>
                                        <bs.Button variant="secondary" size="sm" onClick={dismissInline}>
                                            Cancel
                                        </bs.Button>
                                    </div>
                                );
                            }),
                            title: {
                                title: "Task",
                                callback: "title"
                            },
                            status: {
                                title: "Status",
                                css: { width: '120px' },
                                callback: (item) => {
                                    const colors = { pending: 'warning', 'in-progress': 'info', completed: 'success' };
                                    return <bs.Badge bg={colors[item.status]}>{item.status}</bs.Badge>;
                                }
                            },
                            priority: {
                                title: "Priority",
                                css: { width: '100px' },
                                callback: (item) => {
                                    const colors = { low: 'secondary', medium: 'primary', high: 'danger' };
                                    return <bs.Badge bg={colors[item.priority]}>{item.priority}</bs.Badge>;
                                }
                            }
                        }}
                        items={editableItems}
                        itemCount={editableItems.length}
                        itemsPerPage={10}
                        inlinePlus={inlineEditor}
                    />
                </div>

                <CodeBlock>{`import React, { useState } from 'react';
import Skeleton from "../skeleton.js";
import { Button, Badge, Form, Row, Col } from 'react-bootstrap';

function InlineEditTable() {
    const [items, setItems] = useState([
        { id: 1, title: "Task 1", status: "pending", priority: "high" },
        { id: 2, title: "Task 2", status: "completed", priority: "low" },
        { id: 3, title: "Task 3", status: "in-progress", priority: "medium" }
    ]);

    const inlineEditor = (item, dismissInline) => {
        if (!item) {
            // Add new item form
            return (
                <div>
                    <h6 className="mb-3">Create New Task</h6>
                    <Form onSubmit={(e) => {
                        e.preventDefault();
                        const formData = new FormData(e.target);
                        const newItem = {
                            id: Math.max(...items.map(i => i.id)) + 1,
                            title: formData.get('title'),
                            status: formData.get('status'),
                            priority: formData.get('priority')
                        };
                        setItems([...items, newItem]);
                        dismissInline();
                    }}>
                        <Row className="g-2">
                            <Col md={4}>
                                <Form.Control name="title" placeholder="Title" required />
                            </Col>
                            <Col md={3}>
                                <Form.Select name="status" required>
                                    <option value="pending">Pending</option>
                                    <option value="in-progress">In Progress</option>
                                    <option value="completed">Completed</option>
                                </Form.Select>
                            </Col>
                            <Col md={3}>
                                <Form.Select name="priority" required>
                                    <option value="low">Low</option>
                                    <option value="medium">Medium</option>
                                    <option value="high">High</option>
                                </Form.Select>
                            </Col>
                            <Col md={2}>
                                <Button type="submit" className="w-100" variant="success">
                                    Create
                                </Button>
                            </Col>
                        </Row>
                    </Form>
                </div>
            );
        }

        // Edit existing item form
        return (
            <div>
                <h6 className="mb-3">Edit Task: {item.title}</h6>
                <Form onSubmit={(e) => {
                    e.preventDefault();
                    const formData = new FormData(e.target);
                    const updatedItem = {
                        ...item,
                        title: formData.get('title'),
                        status: formData.get('status'),
                        priority: formData.get('priority')
                    };
                    setItems(items.map(i => i.id === item.id ? updatedItem : i));
                    dismissInline();
                }}>
                    <Row className="g-2">
                        <Col md={4}>
                            <Form.Control name="title" defaultValue={item.title} required />
                        </Col>
                        <Col md={3}>
                            <Form.Select name="status" defaultValue={item.status} required>
                                <option value="pending">Pending</option>
                                <option value="in-progress">In Progress</option>
                                <option value="completed">Completed</option>
                            </Form.Select>
                        </Col>
                        <Col md={3}>
                            <Form.Select name="priority" defaultValue={item.priority} required>
                                <option value="low">Low</option>
                                <option value="medium">Medium</option>
                                <option value="high">High</option>
                            </Form.Select>
                        </Col>
                        <Col md={2}>
                            <Button type="submit" className="w-100" variant="primary">
                                Save
                            </Button>
                        </Col>
                    </Row>
                </Form>
            </div>
        );
    };

    return (
        <Skeleton.Table
            labels={{
                edit: Skeleton.Table.label.update(inlineEditor),
                delete: Skeleton.Table.label.delete((item, dismissInline) => {
                    return (
                        <div className="text-center">
                            <p className="mb-3">Delete task: <strong>{item.title}</strong>?</p>
                            <Button
                                variant="danger"
                                size="sm"
                                className="me-2"
                                onClick={() => {
                                    setItems(items.filter(i => i.id !== item.id));
                                    dismissInline();
                                }}
                            >
                                Confirm Delete
                            </Button>
                            <Button variant="secondary" size="sm" onClick={dismissInline}>
                                Cancel
                            </Button>
                        </div>
                    );
                }),
                title: {
                    title: "Task",
                    callback: "title"
                },
                status: {
                    title: "Status",
                    css: { width: '120px' },
                    callback: (item) => {
                        const colors = { pending: 'warning', 'in-progress': 'info', completed: 'success' };
                        return <Badge bg={colors[item.status]}>{item.status}</Badge>;
                    }
                },
                priority: {
                    title: "Priority",
                    css: { width: '100px' },
                    callback: (item) => {
                        const colors = { low: 'secondary', medium: 'primary', high: 'danger' };
                        return <Badge bg={colors[item.priority]}>{item.priority}</Badge>;
                    }
                }
            }}
            items={items}
            itemCount={items.length}
            itemsPerPage={10}
            inlinePlus={inlineEditor}
        />
    );
}

export default InlineEditTable;`}</CodeBlock>
            </div>

            {/* Example 4: Action Panels */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">4</bs.Badge>Table with Action Panels</h2>
                <p>
                    Add dropdown action panels using <code>somePanels</code> prop. Use <code>Skeleton.Table.actionPanel()</code> 
                    to create organized action menus with headers and dividers.
                </p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <Skeleton.Table
                        labels={{
                            checkbox: Skeleton.Table.label.checkbox(actionItems, actionSelected, setActionSelected),
                            order: {
                                title: "Order #",
                                callback: "order"
                            },
                            customer: {
                                title: "Customer",
                                callback: "customer"
                            },
                            total: {
                                title: "Total",
                                callback: (item) => `$${item.total}`
                            },
                            status: {
                                title: "Status",
                                css: { width: '120px' },
                                callback: (item) => {
                                    const colors = { pending: 'warning', shipped: 'info', delivered: 'success' };
                                    return <bs.Badge bg={colors[item.status]}>{item.status}</bs.Badge>;
                                }
                            }
                        }}
                        items={actionItems}
                        itemCount={actionItems.length}
                        itemsPerPage={10}
                        selectedItems={actionSelected}
                        somePanels={[
                            {
                                label: <><i className="bi bi-filter"></i> Filters</>,
                                width: '300px',
                                content: (
                                    <div className="p-3">
                                        <h6 className="mb-3">Filter Orders</h6>
                                        <bs.Form.Group className="mb-2">
                                            <bs.Form.Label>Status</bs.Form.Label>
                                            <bs.Form.Select size="sm">
                                                <option>All</option>
                                                <option>Pending</option>
                                                <option>Shipped</option>
                                                <option>Delivered</option>
                                            </bs.Form.Select>
                                        </bs.Form.Group>
                                        <bs.Button size="sm" variant="primary" className="w-100">
                                            Apply Filters
                                        </bs.Button>
                                    </div>
                                )
                            },
                            {
                                label: <><i className="bi bi-gear"></i> Actions</>,
                                width: '350px',
                                content: Skeleton.Table.actionPanel({
                                    "Bulk Actions": [
                                        {
                                            label: `Export Selected (${actionSelected.length})`,
                                            icon: "bi bi-download",
                                            callback: () => context.toast.open("Exporting selected items...", { variant: "info", title: "Export" })
                                        },
                                        {
                                            label: "Mark as Shipped",
                                            icon: "bi bi-truck",
                                            callback: () => context.toast.open("Orders marked as shipped", { variant: "success", title: "Updated" })
                                        }
                                    ],
                                    "Table Options": [
                                        {
                                            label: "Refresh",
                                            icon: "bi bi-arrow-clockwise",
                                            callback: () => setActionSelected([])
                                        },
                                        {
                                            label: "Print",
                                            icon: "bi bi-printer",
                                            callback: () => window.print()
                                        }
                                    ],
                                    "Danger Zone": [
                                        {
                                            label: "Delete Selected",
                                            icon: "bi bi-trash",
                                            callback: () => context.alert("This will delete selected orders", { variant: "warning", title: "Warning" })
                                        }
                                    ]
                                })
                            }
                        ]}
                    />
                </div>

                <CodeBlock>{`import React, { useState } from 'react';
import Skeleton from "../skeleton.js";
import { Badge, Form, Button } from 'react-bootstrap';

function ActionPanelTable() {
    const [items, setItems] = useState([
        { id: 1, order: "#1001", customer: "John Doe", total: 150, status: "shipped" },
        { id: 2, order: "#1002", customer: "Jane Smith", total: 250, status: "pending" },
        { id: 3, order: "#1003", customer: "Bob Johnson", total: 99, status: "delivered" }
    ]);
    const [selectedItems, setSelectedItems] = useState([]);

    return (
        <Skeleton.Table
            labels={{
                checkbox: Skeleton.Table.label.checkbox(items, selectedItems, setSelectedItems),
                order: {
                    title: "Order #",
                    callback: "order"
                },
                customer: {
                    title: "Customer",
                    callback: "customer"
                },
                total: {
                    title: "Total",
                    callback: (item) => \`$\${item.total}\`
                },
                status: {
                    title: "Status",
                    css: { width: '120px' },
                    callback: (item) => {
                        const colors = { pending: 'warning', shipped: 'info', delivered: 'success' };
                        return <Badge bg={colors[item.status]}>{item.status}</Badge>;
                    }
                }
            }}
            items={items}
            itemCount={items.length}
            itemsPerPage={10}
            selectedItems={selectedItems}
            somePanels={[
                {
                    label: <><i className="bi bi-filter"></i> Filters</>,
                    width: '300px',
                    content: (
                        <div className="p-3">
                            <h6 className="mb-3">Filter Orders</h6>
                            <Form.Group className="mb-2">
                                <Form.Label>Status</Form.Label>
                                <Form.Select size="sm">
                                    <option>All</option>
                                    <option>Pending</option>
                                    <option>Shipped</option>
                                    <option>Delivered</option>
                                </Form.Select>
                            </Form.Group>
                            <Button size="sm" variant="primary" className="w-100">
                                Apply Filters
                            </Button>
                        </div>
                    )
                },
                {
                    label: <><i className="bi bi-gear"></i> Actions</>,
                    width: '350px',
                    content: Skeleton.Table.actionPanel({
                        "Bulk Actions": [
                            {
                                label: \`Export Selected (\${selectedItems.length})\`,
                                icon: "bi bi-download",
                                callback: () => console.log("Export selected items...")
                            },
                            {
                                label: "Mark as Shipped",
                                icon: "bi bi-truck",
                                callback: () => console.log("Marking as shipped...")
                            }
                        ],
                        "Table Options": [
                            {
                                label: "Refresh",
                                icon: "bi bi-arrow-clockwise",
                                callback: () => setSelectedItems([])
                            },
                            {
                                label: "Print",
                                icon: "bi bi-printer",
                                callback: () => window.print()
                            }
                        ],
                        "Danger Zone": [
                            {
                                label: "Delete Selected",
                                icon: "bi bi-trash",
                                callback: () => console.log("Delete selected...")
                            }
                        ]
                    })
                }
            ]}
        />
    );
}

export default ActionPanelTable;`}</CodeBlock>
            </div>

            {/* Example 5: Standard Pagination */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">5</bs.Badge>Standard Pagination</h2>
                <p>
                    Use <code>usePagination</code> hook for standard page-based navigation.
                    Provides multiple UI components: <code>PrevNext</code>, <code>Buttons</code>, <code>Inputbox</code>, <code>Selectbox</code>.
                </p>
                <p>
                    While <code>isPending</code>, each column renders <code>itemsPerPage</code> placeholder rows by default (or the number of
                    rows already on screen, if there were any — see <code>items.length</code> below) using a generic single-line bar. Give a
                    column an optional <code>placeholder</code> render function to match its real shape instead — important for any column
                    taller than one line (a title + subtitle, an image), otherwise the table's height jumps the moment real data arrives.
                    The <code>product</code> column below demonstrates this.
                </p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <Skeleton.Table
                        labels={{
                            id: { title: "ID", css: { width: '60px' } },
                            product: {
                                title: "Product",
                                // Matches the real cell's shape (a bolded name + muted SKU
                                // line) — wrapped in the same classes as the real content,
                                // not a hardcoded height, so it still lines up if the base
                                // font scale ever changes (no custom CSS — see CODE_CONVENTIONS.md).
                                placeholder: () => (
                                    <div>
                                        <div className="fw-semibold"><span className="placeholder col-6"></span></div>
                                        <div className="text-muted small"><span className="placeholder col-4"></span></div>
                                    </div>
                                ),
                                callback: (item) => (
                                    <div>
                                        <div className="fw-semibold">{item.product}</div>
                                        <div className="text-muted small">SKU-{String(item.id).padStart(4, '0')}</div>
                                    </div>
                                ),
                            },
                            price: { title: "Price", callback: (item) => `$${item.price}` },
                            category: {
                                title: "Category",
                                callback: "category"
                            }
                        }}
                        items={visibleItems5}
                        itemCount={allPaginatedItems.length}
                        itemsPerPage={itemsPerPage5}
                        isPending={isPending5}
                    />
                    
                    <div className="mt-3">
                        <h6 className="text-muted mb-2">Navigation with Buttons:</h6>
                        {pagination5.PrevNext({ children: pagination5.Buttons(), limitSwitch: true })}
                    </div>

                    <div className="mt-3">
                        <h6 className="text-muted mb-2">Navigation with Selectbox:</h6>
                        {pagination5.PrevNext({ children: pagination5.Selectbox(), limitSwitch: false })}
                    </div>

                    <div className="mt-3">
                        <h6 className="text-muted mb-2">Navigation with Inputbox:</h6>
                        {pagination5.PrevNext({ children: pagination5.Inputbox({ text: "GO" }), limitSwitch: true })}
                    </div>
                </div>

                <CodeBlock>{`import React, { useState } from 'react';
import Skeleton from "../skeleton.js";

function ProductTable() {
    // Generate 50 items
    const allItems = Array.from({ length: 50 }, (_, i) => ({
        id: i + 1,
        product: \`Product \${i + 1}\`,
        price: Math.floor(Math.random() * 1000) + 10,
        category: ['Electronics', 'Clothing', 'Food', 'Books'][Math.floor(Math.random() * 4)]
    }));

    const [isPending, setIsPending] = useState(false);
    const [currentPage, setCurrentPage] = useState(1);
    const itemsPerPage = 5;

    const pagination = Skeleton.usePagination({
        isPending,
        currentPosition: currentPage,
        itemCount: allItems.length,
        itemsPerPage,
        onPositionChange: (newPage) => {
            setIsPending(true);
            setCurrentPage(newPage);
            setTimeout(() => setIsPending(false), 300);
        },
        maxButtons: 7
    });

    // Slice items for current page
    const visibleItems = allItems.slice(
        (currentPage - 1) * itemsPerPage,
        currentPage * itemsPerPage
    );

    return (
        <>
            <Skeleton.Table
                labels={{
                    id: { title: "ID", css: { width: '60px' }, callback: "id" },
                    product: {
                        title: "Product",
                        // Optional — shapes the placeholder cell to match this
                        // column's real (two-line) content. Without it, isPending
                        // falls back to a generic single-line bar, which is fine
                        // for single-line columns like "price"/"category" below.
                        placeholder: () => (
                            <div>
                                <div className="fw-semibold"><span className="placeholder col-6"></span></div>
                                <div className="text-muted small"><span className="placeholder col-4"></span></div>
                            </div>
                        ),
                        callback: (item) => (
                            <div>
                                <div className="fw-semibold">{item.product}</div>
                                <div className="text-muted small">SKU-{String(item.id).padStart(4, '0')}</div>
                            </div>
                        ),
                    },
                    price: { title: "Price", callback: (item) => \`$\${item.price}\` },
                    category: { title: "Category", callback: "category" }
                }}
                items={visibleItems}
                itemCount={allItems.length}
                itemsPerPage={itemsPerPage}
                isPending={isPending}
            />
            
            {/* Navigation with Buttons */}
            {pagination.PrevNext({ 
                children: pagination.Buttons(),
                limitSwitch: true
            })}

            {/* Or with Selectbox */}
            {pagination.PrevNext({ 
                children: pagination.Selectbox()
            })}

            {/* Or with Inputbox */}
            {pagination.PrevNext({ 
                children: pagination.Inputbox({ text: "GO" })
            })}
        </>
    );
}

export default ProductTable;`}</CodeBlock>
            </div>

            {/* Example 6: Token-Based Pagination */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">6</bs.Badge>Token-Based Pagination</h2>
                <p>
                    Use <code>Skeleton.usePagination.TokenBased()</code> for cursor/token-based pagination. 
                    Ideal for APIs that return next page tokens instead of page numbers.
                </p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <Skeleton.Table
                        labels={{
                            id: {
                                title: "ID",
                                callback: "id"
                            },
                            name: {
                                title: "Name",
                                callback: "name"
                            },
                            value: { title: "Value", callback: (item) => `${item.value}%` }
                        }}
                        items={tokenItems}
                        itemCount={totalCount6}
                        itemsPerPage={5}
                        isPending={isPending6}
                    />
                    
                    <div className="mt-3">
                        {tokenPagination}
                    </div>

                    <div className="mt-3 text-center">
                        <bs.Button
                            variant="outline-secondary"
                            size="sm"
                            onClick={resetPagination}
                            disabled={isPending6}
                        >
                            <i className="bi bi-arrow-clockwise me-1"></i>
                            Reset Pagination
                        </bs.Button>
                    </div>
                </div>

                <CodeBlock>{`import React, { useState, useEffect } from 'react';
import Skeleton from "../skeleton.js";
import { Button } from 'react-bootstrap';

function TokenPaginatedTable() {
    const [isPending, setIsPending] = useState(false);
    const [items, setItems] = useState([]);

    const [pagination, setPaginationToken, resetPagination] = 
        Skeleton.usePagination.TokenBased({
            isPending,
            onTokenChange: (token) => {
                setIsPending(true);
                
                // Simulate API call
                fetch(\`/api/items?cursor=\${token}\`)
                    .then(res => res.json())
                    .then(data => {
                        setItems(data.items);
                        
                        // Register this page with pagination
                        setPaginationToken(
                            token,              // Current page token
                            data.nextToken,     // Next page token (or null if last page)
                            data.itemsPerPage,  // Items per page
                            data.items.length   // Current page item count
                        );
                        
                        setIsPending(false);
                    });
            },
            limitSwitch: true,
            maxButtons: 7
        });

    // Initialize on mount
    useEffect(() => {
        // Initial load will trigger onTokenChange with null token
    }, []);

    return (
        <>
            <Skeleton.Table
                labels={{
                    id: { title: "ID", callback: "id" },
                    name: { title: "Name", callback: "name" },
                    value: { title: "Value", callback: (item) => \`\${item.value}%\` }
                }}
                items={items}
                itemCount={items.length}
                itemsPerPage={5}
                isPending={isPending}
            />
            
            {/* Render pagination component */}
            {pagination}

            {/* Reset button */}
            <div className="text-center mt-3">
                <Button 
                    variant="outline-secondary" 
                    size="sm" 
                    onClick={resetPagination}
                    disabled={isPending}
                >
                    Reset Pagination
                </Button>
            </div>
        </>
    );
}

export default TokenPaginatedTable;`}</CodeBlock>
            </div>

            {/* Example 7: Skeleton.usePagination.DataTable */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">7</bs.Badge>Skeleton.usePagination.DataTable Helper</h2>
                <p>
                    Use <code>Skeleton.usePagination.DataTable()</code> for quick in-memory pagination. 
                    It automatically slices your items array and returns both pagination component and paginated items.
                </p>

                <div className="mb-3">
                    <h6 className="text-muted mb-3">RESULT</h6>
                    <Skeleton.Table
                        labels={{
                            id: { title: "ID", css: { width: '60px' } },
                            user: {
                                title: "Username",
                                callback: "user"
                            },
                            email: {
                                title: "Email",
                                callback: "email"
                            },
                            joined: {
                                title: "Joined",
                                css: { width: '120px' },
                                callback: "joined"
                            }
                        }}
                        items={paginatedItems7}
                        itemCount={allDataTableItems.length}
                        itemsPerPage={itemsPerPage7}
                        isPending={isPending7}
                    />
                    
                    <div className="mt-3">
                        {pagination7.PrevNext({ 
                            children: pagination7.Buttons(),
                            limitSwitch: true 
                        })}
                    </div>
                </div>

                <CodeBlock>{`import React, { useState } from 'react';
import Skeleton from "../skeleton.js";

function UserTable() {
    // Generate 30 users
    const date = String(Math.floor(Math.random() * 28) + 1);
    const month = String(Math.floor(Math.random() * 12) + 1);
    const allItems = Array.from({ length: 30 }, (_, i) => ({
        id: i + 1,
        user: \`User \${i + 1}\`,
        email: \`user\${i + 1}@example.com\`,
        joined: \`2024-\${month.padStart(2, '0')}-\${date.padStart(2, '0')}\`
    }));

    const [isPending, setIsPending] = useState(false);
    const [currentPage, setCurrentPage] = useState(1);
    const itemsPerPage = 6;

    // DataTable helper returns [pagination, paginatedItems]
    const [pagination, paginatedItems] = Skeleton.usePagination.DataTable({
        isPending,
        items: allItems,                // Full items array
        currentPosition: currentPage,
        itemsPerPage: itemsPerPage,
        onPositionChange: (newPage) => {
            setIsPending(true);
            setCurrentPage(newPage);
            setTimeout(() => setIsPending(false), 300);
        },
        maxButtons: 9
    });

    return (
        <>
            {/* paginatedItems is automatically sliced! */}
            <Skeleton.Table
                labels={{
                    id: { title: "ID", css: { width: '60px' }, callback: "id" },
                    user: { title: "Username", callback: "user" },
                    email: { title: "Email", callback: "email" },
                    joined: { title: "Joined", css: { width: '120px' }, callback: "joined" }
                }}
                items={paginatedItems}
                itemCount={allItems.length}
                itemsPerPage={itemsPerPage}
                isPending={isPending}
            />

            {/* Use pagination components */}
            {pagination.PrevNext({ 
                children: pagination.Buttons(),
                limitSwitch: true 
            })}

            {/* Alternative: just buttons */}
            {/* {pagination.Buttons()} */}

            {/* Or selectbox */}
            {/* {pagination.Selectbox()} */}
        </>
    );
}

export default UserTable;`}</CodeBlock>
            </div>

            {/* API Reference */}
            <div className="mb-5">
                <h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="secondary" className="me-2">API</bs.Badge>API Reference</h2>

                <h4 className="mt-4 mb-3">Table Props</h4>
                <div className="table-responsive mb-4">
                    <table className="table table-bordered">
                        <thead>
                            <tr>
                                <th style={{ width: "25%" }}>Prop</th>
                                <th style={{ width: "20%" }}>Type</th>
                                <th style={{ width: "15%" }}>Default</th>
                                <th>Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td><code>labels</code></td>
                                <td>Object</td>
                                <td><code>{'{}'}</code></td>
                                <td>
                                    Column definitions. Each key is a column, value can be:
                                    <br />
                                    <small className="text-muted">
                                        • String: Column title<br />
                                        • Object: {'{'}title, css, callback{'}'}
                                    </small>
                                </td>
                            </tr>
                            <tr>
                                <td><code>items</code></td>
                                <td>Array</td>
                                <td><code>[]</code></td>
                                <td>Array of data items to display</td>
                            </tr>
                            <tr>
                                <td><code>itemCount</code></td>
                                <td>Number</td>
                                <td><code>0</code></td>
                                <td>Total number of items (for displaying count)</td>
                            </tr>
                            <tr>
                                <td><code>itemsPerPage</code></td>
                                <td>Number</td>
                                <td><code>20</code></td>
                                <td>Number of items per page (affects loading skeleton)</td>
                            </tr>
                            <tr>
                                <td><code>isPending</code></td>
                                <td>Boolean</td>
                                <td><code>false</code></td>
                                <td>Shows loading skeleton when true</td>
                            </tr>
                            <tr>
                                <td><code>selectedItems</code></td>
                                <td>Array</td>
                                <td><code>[]</code></td>
                                <td>Array of selected item IDs (for checkbox state)</td>
                            </tr>
                            <tr>
                                <td><code>inlinePlus</code></td>
                                <td>Function</td>
                                <td><code>null</code></td>
                                <td>
                                    Callback for inline editing/adding. Receives:
                                    <br />
                                    <small className="text-muted">
                                        • item: Item being edited (null for new)<br />
                                        • dismissInline: Function to close inline editor
                                    </small>
                                </td>
                            </tr>
                            <tr>
                                <td><code>somePanels</code></td>
                                <td>Array</td>
                                <td><code>[]</code></td>
                                <td>
                                    Action panel dropdowns in table header. Each panel:
                                    <br />
                                    <small className="text-muted">
                                        • label: React element<br />
                                        • width: Panel width (default: '400px')<br />
                                        • content: React element
                                    </small>
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <h4 className="mt-5 mb-3">Skeleton.Table.label Methods</h4>
                <div className="table-responsive mb-4">
                    <table className="table table-bordered">
                        <thead>
                            <tr>
                                <th style={{ width: "40%" }}>Method</th>
                                <th>Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td><code>Skeleton.Table.label.checkbox(items, selected, setSelected, keyname)</code></td>
                                <td>
                                    Creates checkbox column with select-all header
                                    <br />
                                    <small className="text-muted">
                                        • items: Current page items<br />
                                        • selected: Selected IDs array<br />
                                        • setSelected: State setter<br />
                                        • keyname: ID field name (default: "id")
                                    </small>
                                </td>
                            </tr>
                            <tr>
                                <td><code>Skeleton.Table.label.update(callback)</code></td>
                                <td>
                                    Creates edit icon column
                                    <br />
                                    <small className="text-muted">
                                        • callback: Function(item, dismissInline) returning JSX
                                    </small>
                                </td>
                            </tr>
                            <tr>
                                <td><code>Skeleton.Table.label.delete(callback)</code></td>
                                <td>
                                    Creates delete icon column
                                    <br />
                                    <small className="text-muted">
                                        • callback: Function(item, dismissInline) returning JSX
                                    </small>
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <h4 className="mt-5 mb-3">Skeleton.Table.actionPanel()</h4>
                <div className="table-responsive mb-4">
                    <table className="table table-bordered">
                        <thead>
                            <tr>
                                <th style={{ width: "40%" }}>Method</th>
                                <th>Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td><code>Skeleton.Table.actionPanel(config)</code></td>
                                <td>
                                    Creates action menu with sections
                                    <br />
                                    <small className="text-muted">
                                        • config: Object with section names as keys<br />
                                        • Each section: Array of {'{'} label, icon, callback {'}'}
                                    </small>
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <h4 className="mt-5 mb-3">usePagination Hook</h4>
                <div className="table-responsive mb-4">
                    <table className="table table-bordered">
                        <thead>
                            <tr>
                                <th style={{ width: "30%" }}>Parameter</th>
                                <th style={{ width: "20%" }}>Type</th>
                                <th>Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td><code>isPending</code></td>
                                <td>Boolean</td>
                                <td>Disables pagination controls</td>
                            </tr>
                            <tr>
                                <td><code>currentPosition</code></td>
                                <td>Number</td>
                                <td>Current page number (1-indexed)</td>
                            </tr>
                            <tr>
                                <td><code>itemCount</code></td>
                                <td>Number</td>
                                <td>Total number of items</td>
                            </tr>
                            <tr>
                                <td><code>itemsPerPage</code></td>
                                <td>Number</td>
                                <td>Items per page</td>
                            </tr>
                            <tr>
                                <td><code>onPositionChange</code></td>
                                <td>Function</td>
                                <td>Callback(newPage) when page changes</td>
                            </tr>
                            <tr>
                                <td><code>maxButtons</code></td>
                                <td>Number</td>
                                <td>Number of page buttons (default: 11)</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <h6 className="mt-4 mb-2">Returns Object:</h6>
                <div className="table-responsive mb-4">
                    <table className="table table-bordered">
                        <thead>
                            <tr>
                                <th style={{ width: "35%" }}>Component</th>
                                <th>Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td><code>PrevNext({'{'} children, limitSwitch {'}'})</code></td>
                                <td>Wrapper with prev/next buttons</td>
                            </tr>
                            <tr>
                                <td><code>Buttons()</code></td>
                                <td>Numbered page buttons</td>
                            </tr>
                            <tr>
                                <td><code>Selectbox()</code></td>
                                <td>Dropdown page selector</td>
                            </tr>
                            <tr>
                                <td><code>Inputbox({'{'} text {'}'})</code></td>
                                <td>Manual page input</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <h4 className="mt-5 mb-3">Skeleton.usePagination.TokenBased</h4>
                <div className="table-responsive mb-4">
                    <table className="table table-bordered">
                        <thead>
                            <tr>
                                <th style={{ width: "30%" }}>Parameter</th>
                                <th>Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td><code>isPending</code></td>
                                <td>Boolean - disables controls</td>
                            </tr>
                            <tr>
                                <td><code>onTokenChange</code></td>
                                <td>Function(token) - called when page changes</td>
                            </tr>
                            <tr>
                                <td><code>limitSwitch</code></td>
                                <td>Boolean - show first/last buttons (default: true)</td>
                            </tr>
                            <tr>
                                <td><code>maxButtons</code></td>
                                <td>Number - max page buttons (default: 9)</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <h6 className="mt-4 mb-2">Returns Array: [PaginationComponent, setPaginationToken, resetPagination]</h6>
                <div className="table-responsive mb-4">
                    <table className="table table-bordered">
                        <thead>
                            <tr>
                                <th style={{ width: "30%" }}>Return Value</th>
                                <th>Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td><code>PaginationComponent</code></td>
                                <td>JSX element to render</td>
                            </tr>
                            <tr>
                                <td><code>setPaginationToken</code></td>
                                <td>
                                    Function(currentToken, nextToken, itemsPerPage, itemCount)
                                    <br />
                                    <small className="text-muted">Call after fetching data to register page</small>
                                </td>
                            </tr>
                            <tr>
                                <td><code>resetPagination</code></td>
                                <td>Function() - resets to first page</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <h4 className="mt-5 mb-3">Skeleton.usePagination.DataTable</h4>
                <div className="table-responsive">
                    <table className="table table-bordered">
                        <thead>
                            <tr>
                                <th style={{ width: "30%" }}>Parameter</th>
                                <th>Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td>Same as usePagination</td>
                                <td>Plus: <code>items</code> array (full dataset)</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <h6 className="mt-3 mb-2">Returns Array: [PaginationObject, PaginatedItems]</h6>
                <small className="text-muted">
                    PaginationObject same as usePagination return. PaginatedItems is automatically sliced array.
                </small>

                <div className="alert alert-info border-0 mt-4">
                    <i className="bi bi-info-circle me-2"></i>
                    <strong>Note:</strong> For client-side pagination of static data, use <code>Skeleton.usePagination.DataTable</code>. 
                    For server-side pagination, use <code>usePagination</code> or <code>Skeleton.usePagination.TokenBased</code>.
                </div>
            </div>
        </div>
    );
}