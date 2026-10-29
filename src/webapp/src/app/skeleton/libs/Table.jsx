import React, { useEffect, useState } from 'react';
import * as bs from 'react-bootstrap';

// Local copy of Skeleton.keyActivate — this file is imported BY skeleton.js,
// so it can't import Skeleton back without a circular dependency.
const keyActivate = (onClick) => (e) => {
    if (e.key === 'Enter' || e.key === ' ' || e.key === 'Spacebar') {
        e.preventDefault();
        onClick(e);
    }
};

export function Table({ labels, items, itemCount, isPending, itemsPerPage = 20, selectedItems = [], inlinePlus = null, somePanels = [] }) {
    labels = Object.values(labels).map((label, i) => {
        if (typeof label === 'string') {
            return { title: label, css: {}, callback: null };
        }

        return label;
    });
    const [selectedLine, setSelectedLine] = useState(-1);
    const [inlineComponent, setInlineComponent] = useState(null);
    const dismissInlinePlus = () => {
        setSelectedLine(-1);
        setInlineComponent(null);
    };
    const handleCallback = (callback, item, i) => {
        if (typeof callback == "function")
            return callback(item, (component) => {
                if (!component)
                    return dismissInlinePlus();

                setSelectedLine(item.id || (i + 1));
                setInlineComponent(component(item, dismissInlinePlus, i + 1));
            });

        if (callback in item)
            return item[callback];

        return callback;
    };
    const handlePendingState = () => {
        // On the very first load there's no prior data to reference, so
        // itemsPerPage is the only reasonable guess. On every subsequent
        // fetch (pagination, search, sort...) the table already has real
        // rows on screen — morph those into the same number of placeholder
        // rows instead of resetting to itemsPerPage, since the table can't
        // know the next page's actual row count in advance (a trailing page
        // legitimately returns fewer).
        const placeholderCount = items.length > 0 ? items.length : itemsPerPage;

        return <>{Array.from({ length: placeholderCount }).map((_, i) => {
            return (
                <tr key={i} className="placeholder-glow">
                    {Object.values(labels).map((label, j) => {
                        // A column's placeholder cell should be shaped like that
                        // column's real content (a `label.placeholder` render
                        // function lets the caller say what that is — see e.g.
                        // LiveGames.jsx's "photo"/"title" columns), not a generic
                        // guess — otherwise the row's height changes the moment
                        // real data replaces the placeholder. `label.css` (column
                        // width etc.) is applied here too, same as real cells.
                        return (
                            <td key={j} style={label.css || {}}>
                                {typeof label.placeholder === "function" ? label.placeholder() : <span className="placeholder w-100"></span>}
                            </td>
                        );
                    })}
                </tr>
            );
        })}</>;
    };
    const echoInlineComponent = (colspan, item, i) => {
        return (
            <tr>
                <td colSpan={colspan} className="p-3 text-start bg-info bg-opacity-10">
                    <div className="d-flex justify-content-between align-items-center mb-3 border-bottom pb-2">
                        <h6 className="mb-0">
                            Item: {item ? item?.id || (i + 1) : '...'}
                        </h6>
                        <button
                            className="btn btn-sm btn-outline-secondary pb-1"
                            onClick={dismissInlinePlus}
                        >
                            <i className="bi bi-x-lg"></i>
                        </button>
                    </div>
                    {inlineComponent}
                </td>
            </tr>
        );
    };

    useEffect(() => {
        if (selectedLine != -1 && !inlineComponent) {
            setSelectedLine(-1);
        }

        if (isPending) {
            setSelectedLine(-1);
            setInlineComponent(null);
        }
    }, [isPending, inlineComponent, selectedLine]);

    return (
        <div key={Math.random()}>
            <bs.Row className="mb-3 align-items-center">
                <bs.Col md="9">
                    <h5 className="text-secondary m-0">
                        Showing <b>{items.length}</b> items on over a total of <b>{itemCount}</b> items
                        {selectedItems.length > 0 ? (<> (<b>{selectedItems.length}</b> selected).</>) : "."}
                    </h5>
                </bs.Col>
                <bs.Col md="3" className="text-end text-md-end d-flex justify-content-end align-items-center gap-3">
                    {inlinePlus ? (
                        <h5
                            role="button"
                            tabIndex={0}
                            title="Add New Item"
                            className="text-secondary mb-0"
                            onClick={() => {
                                setSelectedLine(0);
                                setInlineComponent(inlinePlus(null, dismissInlinePlus));
                            }}
                            onKeyDown={keyActivate(() => {
                                setSelectedLine(0);
                                setInlineComponent(inlinePlus(null, dismissInlinePlus));
                            })}>
                            <i className="bi bi-plus-lg"></i>
                        </h5>
                    ) : null}
                    {Array.isArray(somePanels) && somePanels.length > 0 ? somePanels.map((panel, i) => {
                        return (
                            <bs.Dropdown key={i} align="end">
                                <bs.Dropdown.Toggle as={React.forwardRef(({ children, onClick }, ref) => (
                                    <a
                                        href="#"
                                        ref={ref}
                                        onClick={(e) => {
                                            e.preventDefault();
                                            onClick(e);
                                        }}
                                        role="button"
                                        className="text-decoration-none"
                                    >
                                        {children}
                                    </a>
                                ))}>
                                    <h5 className="text-secondary m-0">
                                        {panel.label}
                                    </h5>
                                </bs.Dropdown.Toggle>
                                {panel.content ? (
                                    <bs.Dropdown.Menu
                                        className="dropdown-menu-wide p-0"
                                        style={{ minWidth: panel?.width || '400px' }}
                                        onClick={e => e.stopPropagation()}>
                                        <div key={`panel-content-${i}`}>
                                            {panel.content}
                                        </div>
                                    </bs.Dropdown.Menu>
                                ) : null}
                            </bs.Dropdown>
                        );
                    }) : null}
                </bs.Col>
            </bs.Row>
            <bs.Table
                key={Math.random()}
                responsive
                className={`align-middle border libs-table ${selectedLine == -1 && !inlineComponent ? 'table-hover' : ''}`}>
                <thead key="thead" className="fw-bold">
                    <tr key="0">
                        {Object.values(labels).map((label, i) => {
                            return (<th key={i} style={label.css || {}}>{label.title}</th>);
                        })}
                    </tr>
                </thead>
                <tbody key="tbody">
                    {selectedLine == 0 && inlineComponent ? echoInlineComponent(Object.values(labels).length, null, 0) : null}
                    {isPending ? handlePendingState() : items.length == 0 ? (
                        <tr>
                            <td colSpan={Object.values(labels).length} className="text-center">
                                <div style={{ padding: '40px' }} className="fw-bold">
                                    No items to display.
                                </div>
                            </td>
                        </tr>
                    ) : items.map((item, i) => {
                        const isSelectedLine = item.id === selectedLine || (i + 1) === selectedLine;

                        return (
                            <React.Fragment key={i}>
                                <tr className={isSelectedLine ? 'table-primary' : ''}>
                                    {Object.values(labels).map((label, j) => {
                                        return (<td key={j} style={label.css || {}}>{handleCallback(label.callback || label.title || null, item, i)}</td>);
                                    })}
                                </tr>
                                {isSelectedLine && inlineComponent ? echoInlineComponent(Object.values(labels).length, item, i) : null}
                            </React.Fragment>
                        );
                    })}
                </tbody>
            </bs.Table>
        </div>
    );
}

Table.label = {
    checkbox: (items, selectedItems, setSelectedItems, keyname = "id") => {
        return {
            title: (
                <bs.Form.Check
                    checked={items.length > 0 && items.every(item => selectedItems.includes(item?.[keyname] ?? null))}
                    onChange={(e) => {
                        if (e.target.checked) {
                            setSelectedItems([...selectedItems, ...items.map(item => item?.[keyname] ?? null)]);
                        } else {
                            setSelectedItems(selectedItems.filter(id => !items.map(item => item?.[keyname] ?? null).includes(id)));
                        }
                    }} />
            ),
            css: { width: '30px' },
            callback: (item, _, i) => {
                const itemID = item?.[keyname] ?? i;

                return (
                    <bs.Form.Check
                        checked={selectedItems.includes(itemID)}
                        onChange={(e) => {
                            if (e.target.checked) {
                                setSelectedItems([...selectedItems, itemID]);
                            } else {
                                setSelectedItems(selectedItems.filter(id => id !== itemID));
                            }
                        }}
                    />
                );
            }
        };
    },
    update: (callback) => {
        return {
            title: <i className="bi bi-pencil"></i>,
            css: { width: '30px', textAlign: 'center' },
            callback: (_, onclickShowInline, i) => {
                return (
                    <i
                        key={i}
                        className="bi bi-pencil text-primary"
                        style={{ cursor: 'pointer' }}
                        onClick={(e) => {
                            e.preventDefault();
                            onclickShowInline(callback);
                        }}
                    ></i>
                );
            }
        }
    },
    delete: (callback) => {
        return {
            title: <i className="bi bi-trash"></i>,
            css: { width: '30px', textAlign: 'center' },
            callback: (_, onclickShowInline, i) => {
                return (
                    <i
                        key={i}
                        className="bi bi-trash text-primary"
                        style={{ cursor: 'pointer' }}
                        onClick={(e) => {
                            e.preventDefault();
                            onclickShowInline(callback);
                        }}></i>
                );
            }
        };
    }
};

Table.actionPanel = (actionPanel = {}) => {
    return (
        <div>
            {!Array.isArray(actionPanel) && Object.keys(actionPanel).length > 0 ? Object.keys(actionPanel).map((header, i) => {
                const actions = actionPanel[header];

                return (
                    <div key={i}>
                        {i > 0 ? <div className="dropdown-divider"></div> : null}
                        <h6 className="dropdown-header">{header}</h6>

                        {actions.map((item, j) => {
                            return (
                                <button key={j} className="dropdown-item" type="button" onClick={(e) => item.callback(e)}>
                                    {item.icon ? <i className={`${item.icon} me-2`}></i> : null}
                                    {item.label}
                                </button>
                            );
                        })}
                    </div>
                );
            }) : <div className="p-3">No Actions Available</div>}
        </div>
    );
}