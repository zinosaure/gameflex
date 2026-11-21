import React, { useState } from 'react';
import * as bs from 'react-bootstrap';


export function usePagination({ isPending, currentPosition, itemCount, itemsPerPage, onPositionChange, maxButtons = 11 }) {
    let position = parseInt(currentPosition);

    const maxPage = Math.ceil(parseInt(itemCount) / parseInt(itemsPerPage || 15));
    const createButton = ({ value, label, disabled }) => {
        return {
            value: value,
            label: label || value,
            disabled: disabled || isPending,
        }
    };

    if (position > maxPage)
        position = maxPage;

    if (position < 1)
        position = 1;

    if (maxButtons < 2)
        maxButtons = 2;

    if (maxButtons % 2 == 0)
        maxButtons += 1;

    if (maxButtons > maxPage)
        maxButtons = maxPage;

    const buttons = [];
    const minPosition = createButton({ value: 1, label: null, disabled: position == 1 });
    const maxPosition = createButton({ value: maxPage, label: null, disabled: position == maxPage || maxPage == 0 });
    const nextPosition = createButton({ value: position + 1, label: null, disabled: position + 1 > maxPage });
    const previousPosition = createButton({ value: position - 1, label: null, disabled: position - 1 < 1 });

    for (let i = 0; i < maxButtons; i++) {
        let n = position - i;
        let m = position + i;

        if (m <= maxPage)
            buttons.push(createButton({ value: m, label: m, disabled: m == position }));

        if (n > 0 && n != m)
            buttons.unshift(createButton({ value: n, label: n, disabled: n == position }));

        if (buttons.length >= maxButtons)
            break;
    }

    return {
        PrevNext: ({ children = null, limitSwitch = true }) => {
            return (
                <bs.Row>
                    <bs.Col xs="4" className="text-start">
                        {limitSwitch ? <>
                            <bs.Button
                                onClick={(e) => onPositionChange(minPosition.value)}
                                disabled={minPosition.disabled} className="me-2">
                                <i className="bi bi-chevron-bar-left"></i>
                            </bs.Button>
                        </> : null}
                        <bs.Button
                            onClick={(e) => onPositionChange(previousPosition.value)}
                            disabled={previousPosition.disabled}>
                            <i className="bi bi-chevron-left"></i>
                        </bs.Button>
                    </bs.Col>
                    <bs.Col xs="4" className="text-center">
                        {children}
                    </bs.Col>
                    <bs.Col xs="4" className="text-end">
                        <bs.Button
                            onClick={(e) => onPositionChange(nextPosition.value)}
                            disabled={nextPosition.disabled}>
                            <i className="bi bi-chevron-right"></i>
                        </bs.Button>
                        {limitSwitch ? <>
                            <bs.Button
                                onClick={(e) => onPositionChange(maxPosition.value)}
                                disabled={maxPosition.disabled} className="ms-2">
                                <i className="bi bi-chevron-bar-right"></i>
                            </bs.Button>
                        </> : null}
                    </bs.Col>
                </bs.Row>
            );
        },
        Buttons: () => {
            return <div className="text-center">
                {
                    buttons.map((button, i) => {
                        return <bs.Button
                            key={i}
                            onClick={(e) => onPositionChange(button.value)}
                            disabled={button.disabled}
                            className="mx-1">
                            {button.label}
                        </bs.Button>
                    })
                }
            </div>
        },
        Inputbox: ({ text = "GO" }) => {
            const [nextPosition, setNextPosition] = useState(position);

            return <div className="text-center">
                <bs.Row>
                    <bs.Col xs="8" className="text-start">
                        <bs.Form.Control
                            type="number"
                            min={minPosition.value}
                            max={maxPosition.value}
                            value={nextPosition}
                            onChange={e => setNextPosition(e.target.value)} />
                    </bs.Col>
                    <bs.Col xs="4" className="text-end">
                        <bs.Button className="w-100" onClick={(e) => onPositionChange(parseInt(nextPosition))}>
                            {text}
                        </bs.Button>
                    </bs.Col>
                </bs.Row>
            </div>
        },
        Selectbox: () => {
            return <div className="text-center">
                <bs.Form.Select
                    value={position}
                    onChange={(e) => onPositionChange(parseInt(e.target.value))}>
                    {((min, max) => {
                        let jsx = null;

                        for (let value = min; value <= max; value++)
                            jsx = <>{jsx}<option value={value}>{value}</option></>

                        return jsx;
                    })(minPosition.value, maxPosition.value)}
                </bs.Form.Select>
            </div>
        },
    };
}

// Helper to use token-based pagination (e.g., API cursors)
// Returns [PaginationComponent, setPaginationToken, resetPagination]
usePagination.TokenBased = ({ isPending, onTokenChange, limitSwitch = true, maxButtons = 9 }) => {
    const [pageHistory, setPageHistory] = useState([{ token: null, label: 1 }]);
    const [currentPosition, setCurrentPosition] = useState(0);
    const [pageData, setPageData] = useState({});

    const setPaginationToken = (currentToken, nextToken, itemsPerPage, itemCount) => {
        // Calculer le nouveau total
        const newPageData = { ...pageData, [currentToken]: itemCount };

        // Ajouter la page suivante si elle existe
        if (nextToken) {
            newPageData[nextToken] = 0;
        }

        const newTotal = Object.values(newPageData).reduce((a, b) => a + b, 0);

        // Mettre à jour la page courante dans l'historique
        setPageHistory(prev => {
            const exists = prev.some(page => page.token === currentToken);
            if (!exists) {
                return [...prev, { token: currentToken, label: prev.length + 1 }];
            }
            return prev;
        });

        // Ajouter la page suivante dans l'historique si elle existe
        if (nextToken && !pageHistory.some(page => page.token === nextToken) && itemCount >= itemsPerPage) {
            setPageHistory(prevHistory => {
                const exists = prevHistory.some(page => page.token === nextToken);
                if (!exists) {
                    return [...prevHistory, { token: nextToken, label: prevHistory.length + 1 }];
                }
                return prevHistory;
            });
        }

        // Mettre à jour les données de la page
        setPageData(newPageData);

        // Retourner le total calculé
        return newTotal;
    };

    const handleNavigate = (newPosition) => {
        setCurrentPosition(newPosition);
        onTokenChange(pageHistory[newPosition].token);
    };

    const Pagination = ({ pageHistory, currentPosition, isPending, onNavigate, limitSwitch, maxButtons }) => {
        const navigate = (direction) => {
            let newPosition = currentPosition;

            if (direction === 'next' && currentPosition < pageHistory.length - 1) {
                newPosition = currentPosition + 1;
            } else if (direction === 'prev' && currentPosition > 0) {
                newPosition = currentPosition - 1;
            } else if (direction === 'first') {
                newPosition = 0;
            } else if (direction === 'last') {
                newPosition = pageHistory.length - 1;
            }

            onNavigate(newPosition);
        };

        const goToPageIndex = (pageIndex) => {
            if (pageIndex >= 0 && pageIndex < pageHistory.length) {
                onNavigate(pageIndex);
            }
        };

        const hasNext = currentPosition < pageHistory.length - 1;
        const hasPrevious = currentPosition > 0;

        return (
            <bs.Row>
                <bs.Col xs="3" className="text-start">
                    {limitSwitch ? <>
                        <bs.Button
                            onClick={() => navigate('first')}
                            disabled={isPending || !hasPrevious} className="me-2">
                            <i className="bi bi-chevron-bar-left"></i>
                        </bs.Button>
                    </> : null}
                    <bs.Button
                        onClick={() => navigate('prev')}
                        disabled={isPending || !hasPrevious}>
                        <i className="bi bi-chevron-compact-left"></i>
                    </bs.Button>
                </bs.Col>
                <bs.Col xs="6" className="text-center">
                    {(() => {
                        const totalPages = pageHistory.length;
                        const halfButtons = Math.floor(maxButtons / 2);

                        // Si moins de boutons que maxButtons, afficher tous
                        if (totalPages <= maxButtons) {
                            return pageHistory.map((button, i) => (
                                <bs.Button
                                    key={i}
                                    onClick={() => goToPageIndex(i)}
                                    disabled={isPending || i === currentPosition}
                                    className="mx-1">
                                    {button.label}
                                </bs.Button>
                            ));
                        }

                        // Calculer la plage à afficher
                        let startIndex = Math.max(0, currentPosition - halfButtons);
                        let endIndex = Math.min(totalPages, startIndex + maxButtons);

                        // Ajuster si on est près de la fin
                        if (endIndex - startIndex < maxButtons) {
                            startIndex = Math.max(0, endIndex - maxButtons);
                        }

                        return pageHistory.slice(startIndex, endIndex).map((button, i) => {
                            const actualIndex = startIndex + i;
                            return (
                                <bs.Button
                                    key={actualIndex}
                                    onClick={() => goToPageIndex(actualIndex)}
                                    disabled={isPending || actualIndex === currentPosition}
                                    className="mx-1">
                                    {button.label}
                                </bs.Button>
                            );
                        });
                    })()}
                </bs.Col>
                <bs.Col xs="3" className="text-end">
                    <bs.Button
                        onClick={() => navigate('next')}
                        disabled={isPending || !hasNext}>
                        <i className="bi bi-chevron-compact-right"></i>
                    </bs.Button>
                    {limitSwitch ? <>
                        <bs.Button
                            onClick={() => navigate('last')}
                            disabled={isPending || !hasNext} className="ms-2">
                            <i className="bi bi-chevron-bar-right"></i>
                        </bs.Button>
                    </> : null}
                </bs.Col>
            </bs.Row>
        );
    };

    return [
        <Pagination
            key={`pagination-component-${pageHistory.length}-${currentPosition}-${maxButtons}`}
            pageHistory={pageHistory}
            currentPosition={currentPosition}
            isPending={isPending}
            onNavigate={handleNavigate}
            limitSwitch={limitSwitch}
            maxButtons={maxButtons}
        />,
        setPaginationToken,
        () => {
            setPageHistory([{ token: null, label: 1 }]);
            setCurrentPosition(0);
            setPageData({});
        },
    ];
};

// Helper to use standard pagination with data tables
// Returns [PaginationComponent, PaginatedItems]
usePagination.DataTable = ({ isPending, items, currentPosition, itemsPerPage, onPositionChange, maxButtons = 11 }) => {
    return [
        usePagination({
            isPending,
            currentPosition,
            itemCount: Array.isArray(items) ? items.length : 0,
            itemsPerPage,
            onPositionChange,
            maxButtons,
        }),
        items.slice((currentPosition - 1) * itemsPerPage, currentPosition * itemsPerPage)
    ];
};