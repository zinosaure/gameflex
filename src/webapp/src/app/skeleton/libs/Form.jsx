import React, { useState, useRef, useMemo, useEffect, useId } from "react";
import * as bs from "react-bootstrap";


function uuidv4() {
    return "10000000-1000-4000-8000-100000000000".replace(/[018]/g, c =>
        (+c ^ crypto.getRandomValues(new Uint8Array(1))[0] & 15 >> +c / 4).toString(16)
    );
}

export function Form({ children, props = {} }) {
    return (
        <bs.Form {...props}>
            {children}
        </bs.Form>
    );
}

Form.get = function (context, name) {
    return context?.values?.[name] || undefined;
};

Form.set = function (context, name, value) {
    context.setValues({ ...context.values, [name]: value });
};

export function Ruleset(callback, values, name = null) {
    if (typeof callback !== "function") {
        console.error(`Ruleset for "${name}" should be a function that returns an object in the format { state[int], message[string] }.`);
    } else {
        const evaluation = callback(values);

        if (evaluation && typeof evaluation === "object" && ("state" in evaluation && "message" in evaluation)) {
            const { state, message } = evaluation ? evaluation : {};

            return {
                isHidden: () => state === Ruleset.IS_HIDDEN,
                isInvalid: () => state === Ruleset.IS_INVALID,
                isDisabled: () => state === Ruleset.IS_DISABLED,
                message: message || null,
            };
        }
    }

    return Ruleset.fallback();
};

Ruleset.IS_HIDDEN = 1;
Ruleset.IS_INVALID = 2;
Ruleset.IS_DISABLED = 3;

Ruleset.fallback = function () {
    return { isHidden: () => false, isInvalid: () => false, isDisabled: () => false, message: null };
};

Ruleset.isHidden = function () {
    return { state: Ruleset.IS_HIDDEN, message: null };
};
Ruleset.isInvalid = function (message) {
    return { state: Ruleset.IS_INVALID, message: message };
};
Ruleset.isDisabled = function (message) {
    return { state: Ruleset.IS_DISABLED, message: message };
};

Ruleset.hasFeedbacks = function (context) {
    const rulesetsFailed = {}

    for (let name of Object.keys(context?.rulesets || {})) {
        const callback = context.rulesets[name];

        if (name.endsWith(".nested") && typeof context.rulesets[name] === "function") {
            const basename = name.replace(".nested", "");

            for (let i = 0; i < (context?.values?.[basename] || []).length; i++) {
                const hasFeedbacks = Ruleset.hasFeedbacks({ values: context.values[basename][i], rulesets: callback(i) });

                if (hasFeedbacks) {
                    rulesetsFailed[`${name}[${i}]`] = hasFeedbacks;
                }
            }
        } else {
            const ruleset = Ruleset(callback, context?.values || {}, name);

            if (ruleset.isInvalid()) {
                rulesetsFailed[name] = ruleset.message;
            }
        }
    }

    return Object.keys(rulesetsFailed).length > 0 ? rulesetsFailed : null;
};

Form.isSubmitted = function (context) {
    return Ruleset.hasFeedbacks(context) === null;
};

Form.submit = function (url, context, options = { headers: {}, method: "POST" }, submitId = uuidv4()) {
    submitId = submitId || uuidv4();
    const formData = new FormData();

    for (const name in context.values) {
        if (!(name in context.files)) {
            const value = context.values[name];

            formData.append(name, (typeof value === 'object' && value !== null) ? JSON.stringify(value) : value);
        }
    }

    return Promise.all([
        Form.submit.PostRequest(url, formData, options, submitId, null),
        ...Form.submit.FilesRequest(url, context, submitId)]).then(responses => {
            return {
                __SKELETON_SUBMIT_ID__: submitId,
                responses: responses.filter(response => response.__SKELETON_SUBMIT_ID__ == submitId).map(response => {
                    delete response.__SKELETON_SUBMIT_ID__;
                    return response;
                }),
            };
        });
};

Form.upload = function (url, context, submitId = uuidv4()) {
    return Promise.all(Form.submit.FilesRequest(url, context, submitId || uuidv4()));
};

Form.submit.PostRequest = function (url, data, options = { headers: {}, method: "POST" }, submitId = uuidv4(), onProgress = null) {
    submitId = submitId || uuidv4();

    if (typeof options !== "object") {
        options = { headers: {}, method: "POST" };
    }

    if (!("headers" in options)) {
        options.headers = {};
    }

    if (!("method" in options)) {
        options.method = "POST";
    }

    return new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();

        xhr.open(options.method || "POST", `${url}${url.includes('?') ? '&' : '?'}submit_id=${submitId}`);

        for (const name in options.headers) {
            if (options.headers[name] !== null) {
                xhr.setRequestHeader(name, options.headers[name]);
            }
        }

        if (onProgress) {
            xhr.upload.addEventListener('progress', onProgress);
        }

        const handleResponse = (code, responseText) => {
            let response = null;

            try {
                response = responseText ? JSON.parse(responseText) : null;
            } catch (e) {
                response = responseText;
            }

            return { __SKELETON_SUBMIT_ID__: submitId, code, response, responseType: typeof response === "object" ? "json" : "text" };
        };

        xhr.onerror = () => {
            reject(handleResponse(xhr.status, xhr.responseText));
        };
        xhr.onload = () => {
            resolve(handleResponse(xhr.status, xhr.responseText));
        };
        xhr.send(data);
    });
};

Form.submit.FilesRequest = function (url, context, submitId = uuidv4()) {
    submitId = submitId || uuidv4();

    return Object.keys(context.files).filter(name => context.files[name].length > 0).flatMap(name =>
        context.files[name].map((file, i) => {
            const formData = new FormData();

            formData.append(name, file);

            return Form.submit.PostRequest(`${url}${url.includes('?') ? '&' : '?'}i=${i}`, formData, { method: "POST", headers: {} }, submitId, (e) => {
                if (!e.lengthComputable) {
                    return;
                }

                context.setValues(prev => ({
                    ...prev, [name]: (prev[name] || []).map((file, j) => {
                        return j === i ? { ...file, pValue: Math.round((e.loaded / e.total) * 100) } : file;
                    }),
                }));
            });
        })
    );
};

Form.Group = function ({ label = null, helpText = null, hidden = () => false, props = { style: { marginBottom: '1rem' } }, children }) {
    if (typeof hidden === "function" && hidden()) {
        return null;
    }

    return (
        <bs.Form.Group {...props}>
            {label || helpText ? (
                <div className="mb-2">
                    {label && (
                        <bs.Form.Label className="fw-bold mb-0">{label}</bs.Form.Label>
                    )}
                    {helpText && (
                        <bs.Form.Text className="text-muted d-block" style={{ marginTop: '-1px' }}>{helpText}</bs.Form.Text>
                    )}
                </div>
            ) : null}
            {children}
        </bs.Form.Group>
    );
};

Form.Feedback = function ({ children, message = null, isFeedbackOnLeft = false, arrowPositions = {}, props = {} }) {
    const [isFeedbackVisible, setFeedbackVisible] = useState();

    return (
        <div
            onMouseEnter={() => setFeedbackVisible(true)}
            onMouseLeave={() => setFeedbackVisible(false)}
            {...props}
        >
            <div className="position-relative">
                {message && (
                    <div style={isFeedbackVisible ? { display: 'block' } : { display: 'none' }}>
                        <div style={{
                            position: 'absolute',
                            bottom: '-2px',
                            right: '11px',
                            width: '15px',
                            height: '15px',
                            backgroundColor: 'var(--bs-danger)',
                            transform: 'rotate(45deg)',
                            zIndex: 1000,
                            ...isFeedbackOnLeft ? { left: '18px', right: 'auto', bottom: '2px' } : {},
                            ...arrowPositions,
                        }}></div>
                        <div style={{
                            bottom: '5px',
                            right: 0,
                            backgroundColor: 'var(--bs-danger)',
                            padding: '3px 10px 6px 10px',
                            borderRadius: 'var(--bs-border-radius-sm)',
                            position: 'absolute',
                            maxWidth: '100%',
                            textAlign: 'right',
                            zIndex: 999,
                            ...isFeedbackOnLeft ? { left: 0, right: 'auto', textAlign: 'left', bottom: '9px' } : {},
                        }}>
                            <bs.Form.Control.Feedback
                                type="invalid"
                                style={{
                                    textAlign: 'left',
                                    color: 'var(--bs-white)',
                                    fontWeight: 'bold',
                                    display: message ? 'block' : 'none',
                                }}>
                                {message}
                            </bs.Form.Control.Feedback>
                        </div>
                    </div>
                )}
            </div>
            {children}
        </div>
    );
}

Form.Input = function ({ name, context, type = "text", isFeedbackOnLeft = false, props = {}, children = null }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={isFeedbackOnLeft}>
            <bs.Form.Control
                type={type}
                name={name}
                value={context?.values?.[name] || ""}
                disabled={ruleset.isDisabled()}
                isInvalid={ruleset.isInvalid()}
                onChange={e => context.setValues({ ...context.values, [name]: e.target.value })}
                {...props}
            />
            {children}
        </Form.Feedback>
    );
};

Form.Text = function ({ name, context, props = { autoComplete: "off" } }) {
    return (
        <Form.Input
            name={name}
            context={context}
            type="text"
            props={props}
        />
    );
};

Form.SearchText = function ({ name, context, freeText = true, callback = null, props = { autoComplete: "off" } }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    }

    const listgroup = useRef(null);
    const [options, setOptions] = useState([]);
    const [searchTerm, setSearchTerm] = useState(() => context?.values?.[name] || "");
    const [selectedItem, setSelectedItem] = useState(-1);

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={false}>
            <div style={{ position: 'relative' }}>
                <bs.Form.Control
                    type="text"
                    name={name}
                    value={searchTerm}
                    disabled={ruleset.isDisabled()}
                    isInvalid={ruleset.isInvalid()}
                    onChange={e => {
                        setSearchTerm(e.target.value);

                        if (freeText) {
                            context.setValues({ ...context.values, [name]: e.target.value });
                        }
                    }}
                    onKeyUp={e => {
                        if (!["ArrowDown", "ArrowUp", "Enter"].includes(e.key)) {
                            if (typeof callback === "function") {
                                setOptions(callback(e.target.value));
                            } else {
                                setOptions([]);
                            }
                        }
                    }}
                    onKeyDown={e => {
                        if (e.key === "ArrowDown" && options.length > 0) {
                            setSelectedItem(selectedItem < options.length - 1 ? selectedItem + 1 : 0);
                            listgroup.current.children[selectedItem < options.length - 1 ? selectedItem + 1 : 0].scrollIntoView({ block: 'nearest' });
                        } else if (e.key === "ArrowUp" && options.length > 0) {
                            setSelectedItem(selectedItem > 0 ? selectedItem - 1 : options.length - 1);
                            listgroup.current.children[selectedItem > 0 ? selectedItem - 1 : options.length - 1].scrollIntoView({ block: 'nearest' });
                        } else if (e.key === "Enter" && selectedItem >= 0 && selectedItem < options.length) {
                            setSearchTerm(options[selectedItem].value);
                            context.setValues({ ...context.values, [name]: options[selectedItem].value });
                            setOptions([]);
                            setSelectedItem(-1);
                        }
                    }}
                    style={{ paddingRight: '40px' }}
                    {...props}
                />
                <i
                    className="bi bi-search position-absolute"
                    style={{
                        top: '50%',
                        right: '15px',
                        transform: 'translateY(-50%)',
                    }}></i>
                <bs.ListGroup
                    ref={listgroup}
                    style={{
                        position: 'absolute',
                        zIndex: 1003,
                        width: '100%',
                        maxHeight: '200px',
                        overflowY: 'auto',
                        display: options.length > 0 ? 'block' : 'none',
                    }}>
                    {options.map((option, i) => (
                        <bs.ListGroup.Item
                            key={i}
                            action
                            onClick={() => {
                                setSearchTerm(option.value);
                                context.setValues({ ...context.values, [name]: option.value });
                                setOptions([]);
                                setSelectedItem(-1);
                            }}
                            style={selectedItem === i ? {
                                backgroundColor: 'var(--bs-primary)',
                                color: '#fff',
                            } : {}}
                        >
                            {option.label}
                        </bs.ListGroup.Item>
                    ))}
                </bs.ListGroup>
            </div>
        </Form.Feedback>
    );
};

Form.IntFloat = function ({ name, context, min = 0, max = 100, step = 1, props = { autoComplete: "off" } }) {
    return (
        <Form.Input
            name={name}
            context={context}
            type="number"
            props={{
                min: min,
                max: max,
                step: step,
                ...props,
            }}
        />
    );
};

Form.Password = function ({ name, context, withStrengthEvaluation = false, props = { autoComplete: "off" } }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    const [isPasswordVisible, setPasswordVisible] = useState(false);
    let strengthEvaluation = null;

    if (withStrengthEvaluation) {
        strengthEvaluation = <Form.Password.StrengthEvaluation name={`${name}_strength`} following={name} context={context} />;
    }

    return (
        <div>
            <div style={{ position: 'relative' }}>
                <Form.Input
                    name={name}
                    context={context}
                    type={isPasswordVisible ? "text" : "password"}
                    props={{
                        style: { paddingLeft: "55px" },
                        ...props,
                    }}
                >
                    <i
                        className={`bi ${isPasswordVisible ? "bi-eye-slash" : "bi-eye"} position-absolute`}
                        style={{
                            top: '50%',
                            left: '15px',
                            transform: 'translateY(-50%)',
                            cursor: 'pointer',
                            paddingRight: '12px',
                            borderRight: '1px dashed var(--bs-border-color)',
                        }}
                        onClick={() => setPasswordVisible(!isPasswordVisible)}
                    ></i>
                </Form.Input>
            </div>
            {strengthEvaluation}
        </div>
    );
};

Form.Password.StrengthEvaluation = function ({ name, following, context, acceptance = 60, tests = [
    { regex: /(.){8,}/, message: "At least 8 characters" },
    { regex: /[A-Z]/, message: "At least one uppercase letter" },
    { regex: /[a-z]/, message: "At least one lowercase letter" },
    { regex: /[0-9]/, message: "At least one number" },
    { regex: /[^A-Za-z0-9]/, message: "At least one special character" },
] }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    }

    const [passwordStrength, setPasswordStrength] = useState(0);
    const checkPasswordStrength = (password) => {
        if (!password || password.length === 0) {
            return setPasswordStrength(0);
        }

        // Pool de caractères utilisés → taille de l'espace des possibles
        let pool = 0;

        if (/[a-z]/.test(password)) pool += 26;
        if (/[A-Z]/.test(password)) pool += 26;
        if (/[0-9]/.test(password)) pool += 10;
        if (/[^A-Za-z0-9]/.test(password)) pool += 32;

        // Entropie de base : longueur × log2(pool)
        const entropy = password.length * Math.log2(pool || 1);

        // Pénalité : caractères répétés réduisent l'entropie réelle
        const uniqueRatio = new Set(password).size / password.length;

        // Pénalité : séquences consécutives (abc, 123, etc.)
        let sequentialCount = 0;

        for (let i = 0; i < password.length - 2; i++) {
            const a = password.charCodeAt(i);
            const b = password.charCodeAt(i + 1);
            const c = password.charCodeAt(i + 2);

            if (b === a + 1 && c === b + 1) {
                sequentialCount++;
            }
        }

        const sequentialPenalty = 1 - (sequentialCount / Math.max(1, password.length - 2)) * 0.5;
        // Entropie ajustée
        const adjustedEntropy = entropy * uniqueRatio * sequentialPenalty;

        // Normalisation : ~80 bits = 100%
        setPasswordStrength(Math.min(100, Math.round((adjustedEntropy / 80) * 100)));
    };

    useEffect(() => {
        if ((context?.values?.[following] || "") !== "") {
            checkPasswordStrength(context?.values?.[following]);
            context.setValues({ ...context.values, [name]: passwordStrength });
        } else {
            setPasswordStrength(0);
        }
    }, [context?.values?.[following]]);

    return (
        <>
            <div className="d-flex justify-content-between gap-2 mt-2" style={{ marginTop: '0.5rem' }}>
                <div style={{
                    flex: 1,
                    height: '5px',
                    backgroundColor: 'var(--bs-border-color)',
                    borderRadius: 'var(--bs-border-radius)',
                    overflow: 'hidden'
                }}>
                    <div style={{
                        width: `${passwordStrength}%`,
                        height: '100%',
                        borderRadius: 'var(--bs-border-radius)',
                        backgroundColor: passwordStrength >= 95 ? 'var(--bs-success)' : passwordStrength >= acceptance ? 'var(--bs-warning)' : 'var(--bs-danger)',
                        transition: 'width 300ms, background-color 300ms',
                    }}></div>
                </div>
            </div>
            <div className="mt-2" style={{ fontSize: '0.925rem' }}>
                {tests.map((item, i) => {
                    const isPassed = item.regex.test(context?.values?.[following] || "");

                    return (
                        <div key={i} style={isPassed ? { color: 'var(--bs-success)', fontWeight: 'bold' } : { color: 'var(--bs-danger)' }}>
                            <div style={{ display: 'inline-block', width: '20px' }}>
                                {isPassed ? <i className="bi bi-check-circle"></i> : <i className="bi bi-x-circle"></i>}
                            </div>
                            <span>{item.message}</span>
                        </div>
                    );
                })}
            </div>
        </>
    );
};

Form.Calendar = function ({ name, context, minDate = null, maxDate = null, isRange = false, selectedDates = [null, null], children = null }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    }

    if (!minDate) {
        minDate = new Date("1900-01-01");
    } else if (typeof minDate === "string") {
        minDate = new Date(minDate);
    }

    if (!maxDate) {
        maxDate = new Date("2099-12-31");
    } else if (typeof maxDate === "string") {
        maxDate = new Date(maxDate);
    }

    if (minDate > maxDate) {
        maxDate = new Date();
    }

    const months = Array.from({ length: 12 }, (_, i) => new Date(0, i).toLocaleString(navigator.language, { month: 'long' }));
    const weekdays = Array.from({ length: 7 }, (_, i) => new Date(2021, 0, i + 3).toLocaleString(navigator.language, { weekday: 'narrow' }));
    const formatDate = (yyyy, mm, dd) => `${yyyy}-${String(mm).padStart(2, '0')}-${String(dd).padStart(2, '0')}`;
    const dateOf = (yyyy, mm, week, weekday) => {
        const firstDayOfMonth = new Date(yyyy, mm - 1, 1).getDay();
        const date = week * 7 + weekday - firstDayOfMonth + 1;

        return new Date(yyyy, mm - 1, date).getMonth() === mm - 1 ? date : "";
    };

    const today = new Date();
    let initialDate = context?.values?.[name] ? new Date(context.values[name]) : today;

    if (initialDate < minDate) {
        initialDate = minDate;
    } else if (initialDate > maxDate) {
        initialDate = maxDate;
    }

    const [pickupDate, setPickupDate] = useState({
        DD: initialDate.getDate(),
        MM: initialDate.getMonth() + 1,
        YYYY: initialDate.getFullYear(),
    });

    let YYYY = Array.from({ length: maxDate.getFullYear() - minDate.getFullYear() + 1 }, (_, i) => ({
        label: maxDate.getFullYear() - i,
        value: maxDate.getFullYear() - i,
        disabled: false,
    }));
    let MM = Array.from({ length: 12 }, (_, i) => ({
        label: months[i],
        value: i + 1,
        disabled: false,
    }));
    let DD = Array.from({ length: 31 }, (_, i) => ({
        label: i + 1,
        value: i + 1,
        disabled: false,
    }));

    if ([4, 6, 9, 11].includes(pickupDate.MM)) {
        DD.splice(30);
    } else if (pickupDate.MM === 2) {
        DD.splice((pickupDate.YYYY % 4 === 0 && pickupDate.YYYY % 100 !== 0) || (pickupDate.YYYY % 400 === 0) ? 29 : 28);
    }

    if (pickupDate.YYYY == maxDate.getFullYear()) {
        MM = MM.map((option, i) => ({
            ...option,
            disabled: i > maxDate.getMonth()
        }));

        if (pickupDate.MM == maxDate.getMonth() + 1) {
            DD = DD.map((option, i) => ({
                ...option,
                disabled: i > maxDate.getDate() - 1
            }));
        }
    }

    if (pickupDate.YYYY == minDate.getFullYear()) {
        MM = MM.map((option, i) => ({
            ...option,
            disabled: i < minDate.getMonth()
        }));

        if (pickupDate.MM == minDate.getMonth() + 1) {
            DD = DD.map((option, i) => ({
                ...option,
                disabled: i < minDate.getDate() - 1
            }));
        }
    }

    const [mouseoverVisible, setMouseoverVisible] = useState(null);

    useEffect(() => {
        setPickupDate(prev => {
            let YYYY = Number(prev.YYYY);
            let MM = Number(prev.MM);
            const DD = prev.DD;

            if (YYYY > maxDate.getFullYear()) {
                YYYY = maxDate.getFullYear();
                MM = maxDate.getMonth() + 1;
            } else if (YYYY === maxDate.getFullYear() && MM > maxDate.getMonth() + 1) {
                MM = maxDate.getMonth() + 1;
            }

            if (YYYY < minDate.getFullYear()) {
                YYYY = minDate.getFullYear();
                MM = minDate.getMonth() + 1;
            } else if (YYYY === minDate.getFullYear() && MM < minDate.getMonth() + 1) {
                MM = minDate.getMonth() + 1;
            }

            return { DD, MM, YYYY };
        });
    }, [minDate.getTime(), maxDate.getTime()]);

    return (
        <>
            <div className="d-flex gap-2 align-items-center justify-content-between">
                <bs.Form.Select
                    value={pickupDate.YYYY}
                    disabled={ruleset.isDisabled()}
                    onChange={e => setPickupDate({ ...pickupDate, YYYY: e.target.value })}
                    style={{ width: '85px' }}
                >
                    {YYYY.map((option, i) => (
                        <option key={i} value={option.value} disabled={option.disabled}>{option.label}</option>
                    ))}
                </bs.Form.Select>
                <bs.Form.Select
                    value={pickupDate.MM}
                    disabled={ruleset.isDisabled()}
                    onChange={e => setPickupDate({ ...pickupDate, MM: e.target.value })}
                >
                    {MM.map((option, i) => {
                        let isDisabled = option.disabled;

                        if (!isDisabled && pickupDate.YYYY == maxDate.getFullYear()) {
                            isDisabled = option.value > maxDate.getMonth() + 1;
                        }
                        if (!isDisabled && pickupDate.YYYY == minDate.getFullYear()) {
                            isDisabled = option.value < minDate.getMonth() + 1;
                        }
                        return <option key={i} value={option.value} disabled={isDisabled}>{months[option.value - 1]}</option>
                    })}
                </bs.Form.Select>
            </div>
            <div className="border-top mt-2 pt-2"></div>
            <div className="d-flex flex-column gap-1 justify-content-between">
                {[0, 1, 2, 3, 4, 5, 6].map(week => (
                    <div key={week} className="d-flex align-items-center justify-content-between">
                        {week === 0 ? weekdays.map((weekday, i) => (
                            <div
                                key={i}
                                className="d-flex align-items-center justify-content-center p-2"
                                style={{
                                    width: isRange ? '22px' : '26px',
                                    height: isRange ? '20px' : '26px',
                                    fontWeight: 'bold',
                                    fontSize: isRange ? '0.7rem' : '0.875rem',
                                    color: 'var(--bs-secondary-color)',
                                }}>
                                {weekday}
                            </div>
                        )) : weekdays.map((weekday, i) => {
                            const date = dateOf(pickupDate.YYYY, pickupDate.MM, week - 1, i);

                            let isActive = false;
                            let isDisabled = ruleset.isDisabled() || DD.find(d => d.value === date)?.disabled;

                            const contextValue = formatDate(pickupDate.YYYY, pickupDate.MM, date);
                            const calendarDate = new Date(contextValue);

                            if ((minDate && calendarDate < minDate) || (maxDate && calendarDate > maxDate)) {
                                isDisabled = true;
                            }

                            if (selectedDates[0] && selectedDates[1]) {
                                if (calendarDate >= new Date(selectedDates[0]) && calendarDate <= new Date(selectedDates[1])) {
                                    isActive = true;
                                }
                            }

                            return (
                                <div
                                    key={i}
                                    className="d-flex align-items-center justify-content-center rounded"
                                    style={{
                                        width: isRange ? '22px' : '32px',
                                        height: isRange ? '20px' : '26px',
                                        fontSize: isRange ? '0.7rem' : '0.875rem',
                                        cursor: isDisabled ? 'not-allowed' : date ? 'pointer' : 'default',
                                        color: isDisabled ? 'var(--bs-tertiary-color)' : 'var(--bs-body-color)',
                                        borderRadius: 'var(--bs-border-radius)',
                                        transition: 'background-color 150ms, color 150ms',
                                        ... (isActive || mouseoverVisible == date || context?.values?.[name] == contextValue ? {
                                            color: 'var(--bs-light)',
                                            backgroundColor: 'var(--bs-primary)',
                                            fontWeight: 'bold',
                                        } : {}),
                                    }}
                                    onClick={() => {
                                        if (isDisabled || !date) {
                                            return null;
                                        }

                                        context.setValues({ ...context.values, [name]: formatDate(pickupDate.YYYY, pickupDate.MM, date) });
                                    }}
                                    onMouseEnter={e => {
                                        if (isDisabled || !date) {
                                            return null;
                                        }
                                        setMouseoverVisible(date);
                                    }}
                                    onMouseLeave={e => {
                                        if (isDisabled || !date) {
                                            return null;
                                        }
                                        setMouseoverVisible(null);
                                    }}
                                >
                                    {date}
                                </div>
                            );
                        })}
                    </div>
                ))}
            </div>
            {children}
            <div className="border-top mt-2 pt-2"></div>
            <div
                className="d-flex align-items-center justify-content-between"
                style={isRange ? { fontSize: '0.7rem' } : { fontSize: '0.875rem' }}>
                <span
                    style={{
                        display: 'block',
                        color: 'var(--bs-secondary-color)',
                        textTransform: 'capitalize',
                        cursor: context?.values?.[name] ? 'pointer' : 'default',
                    }}
                    onClick={() => {
                        if (!context?.values?.[name] || ruleset.isDisabled()) {
                            return null;
                        }

                        setPickupDate(prev => ({
                            ...prev,
                            MM: new Date(context.values[name]).getMonth() + 1,
                            YYYY: new Date(context.values[name]).getFullYear(),
                        }));
                    }}
                >
                    <i className="bi bi-calendar me-2"></i>
                    <b>{context?.values?.[name] ? new Date(context.values[name]).toLocaleDateString(navigator.language, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' }) : '???'}</b>
                </span>
                <span
                    style={{
                        display: 'block',
                        color: 'var(--bs-danger-color)',
                        cursor: context?.values?.[name] ? 'pointer' : 'default',
                    }}
                    onClick={() => {
                        if (!context?.values?.[name] || ruleset.isDisabled()) {
                            return null;
                        }

                        setPickupDate({
                            DD: Math.min(today.getDate(), maxDate.getDate()),
                            MM: Math.min(today.getMonth() + 1, maxDate.getMonth() + 1),
                            YYYY: Math.min(today.getFullYear(), maxDate.getFullYear()),
                        });
                        context.setValues({ ...context.values, [name]: null });
                    }}
                >
                    {context?.values?.[name] ? <i className="bi bi-trash"></i> : null}
                </span>
            </div>
        </>
    );
};

Form.Date = function ({ name, context, minDate = null, maxDate = null, styles = { width: '100%', maxWidth: '300px' }, children = null }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            <div
                className={`border rounded p-2 ${ruleset.isInvalid() ? 'border-danger' : ''}`}
                style={styles}
            >
                <Form.Calendar
                    name={name}
                    context={context}
                    ruleset={ruleset}
                    minDate={minDate}
                    maxDate={maxDate}
                >
                    {children}
                </Form.Calendar>
            </div>
        </Form.Feedback>
    );
};

Form.DateRange = function ({ name, context, minDate = null, maxDate = null, styles = { width: '100%', maxWidth: '768px', overflowX: 'auto' } }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    const dateComponents = Form.DateRange.components({ name, context });

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            <div className={`border rounded p-2 ${ruleset.isInvalid() ? 'border-danger' : ''}`} style={styles}>
                <div className="d-flex gap-2">
                    <div style={{ width: '50%' }}>
                        <dateComponents.Left
                            minDate={minDate}
                            maxDate={maxDate}
                        />
                    </div>
                    <div className="border-start"></div>
                    <div style={{ width: '50%' }}>
                        <dateComponents.Right
                            minDate={minDate}
                            maxDate={maxDate}
                        />
                    </div>
                </div>
            </div>
        </Form.Feedback>
    );
};

Form.DateRange.components = function ({ name, context }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined || !Array.isArray(context?.values?.[name]) || context?.values?.[name].length !== 2) {
        context.values[name] = [null, null];
    }

    return {
        Left: ({ minDate = null, maxDate = null, children = null }) => (
            <Form.Calendar
                name={name}
                context={{
                    ...context,
                    values: {
                        ...context.values,
                        [name]: context.values[name][0] || null,
                    },
                    setValues: (newValues) => {
                        context.setValues({ ...context.values, [name]: [newValues[name], context.values[name][1] || null] });
                    }
                }}
                ruleset={ruleset}
                minDate={minDate}
                maxDate={maxDate || context.values[name][1] || null}
                isRange={true}
                selectedDates={context?.values?.[name]}
            >
                {children}
            </Form.Calendar>
        ),
        Right: ({ minDate = null, maxDate = null, children = null }) => (
            <Form.Calendar name={name}
                context={{
                    ...context,
                    values: {
                        ...context.values,
                        [name]: context.values[name][1] || null,
                    },
                    setValues: (newValues) => {
                        context.setValues({ ...context.values, [name]: [context.values[name][0] || null, newValues[name]] });
                    }
                }}
                ruleset={ruleset}
                minDate={minDate || context.values[name][0] || null}
                maxDate={maxDate}
                isRange={true}
                selectedDates={context?.values?.[name]}
            >
                {children}
            </Form.Calendar>
        ),
    };
};

Form.ClockTime = function ({ name, context, minTime = null, maxTime = null, PM = false }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context?.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    } else if (typeof context.values[name] === "string") {
        const hs = context.values[name].split(':');

        if (hs.length === 2) {
            context.values[name] = `${hs[0].padStart(2, '0')}:${hs[1].padStart(2, '0')}`;
        } else {
            context.values[name] = null;
        }
    } else if (context.values[name] !== null && typeof context.values[name] === "object") {
        context.values[name] = null;
    }

    const [h, setH] = useState(String(context.values[name]).split(':')[0] || "00");
    const [m, setM] = useState(String(context.values[name]).split(':')[1] || "00");

    const hours = Array.from({ length: 24 }, (_, i) => ({
        label: PM ? `${i % 12 || 12} ${i < 12 ? 'AM' : 'PM'}` : String(i).padStart(2, '0'),
        value: String(i).padStart(2, '0'),
        disabled: (minTime && i < Number(minTime.split(':')[0])) || (maxTime && i > Number(maxTime.split(':')[0])),
    }));
    const minutes = Array.from({ length: 60 }, (_, i) => ({
        label: String(i).padStart(2, '0'),
        value: String(i).padStart(2, '0'),
        disabled: false,
    }));

    return (
        <div className="d-flex gap-2 align-items-center justify-content-start w-100">
            <bs.Form.Select
                name={name}
                value={h}
                disabled={ruleset.isDisabled()}
                isInvalid={ruleset.isInvalid()}
                onChange={e => {
                    setH(e.target.value);
                    context.setValues({ ...context.values, [name]: `${e.target.value || "00"}:${m || "00"}` });
                }}
            >
                {hours.map((hour, i) => (
                    <option
                        key={i}
                        value={hour.value}
                        disabled={hour.disabled}>
                        {hour.label}
                    </option>
                ))}
            </bs.Form.Select>
            <span style={{ fontSize: '1.25rem', color: 'var(--bs-secondary-color)' }}>:</span>
            <bs.Form.Select
                name={name}
                value={m}
                disabled={ruleset.isDisabled()}
                isInvalid={ruleset.isInvalid()}
                onChange={e => {
                    const h = context.values[name] ? String(context.values[name]).split(':')[0] : null;

                    if (!h) {
                        setH("00");
                    }

                    setM(e.target.value);
                    context.setValues({ ...context.values, [name]: `${h || "00"}:${e.target.value || "00"}` });
                }}
            >
                {minutes.map((minute, i) => (
                    <option
                        key={i}
                        value={minute.value}
                        disabled={minute.disabled}>
                        {minute.label}
                    </option>
                ))}
            </bs.Form.Select>
        </div>
    );
};

Form.Time = function ({ name, context, minTime = null, maxTime = null, PM = false, styles = { width: '100%', maxWidth: '200px' } }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context?.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            <div style={styles}>
                <Form.ClockTime
                    name={name}
                    context={context}
                    minTime={minTime}
                    maxTime={maxTime}
                    PM={PM}
                />
            </div>
        </Form.Feedback>
    );
};

Form.TimeRange = function ({ name, context, minTime = null, maxTime = null, PM = false, styles = { width: '100%', maxWidth: '440px' } }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context?.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    const components = Form.TimeRange.components({ name, context });

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            <div className="d-flex gap-2 align-items-center justify-content-start" style={styles}>
                <components.Left
                    minTime={minTime}
                    maxTime={maxTime}
                    PM={PM} />
                <span style={{ fontSize: '1.25rem', color: 'var(--bs-secondary-color)' }}>
                    <i className="bi bi-arrow-right"></i>
                </span>
                <components.Right
                    minTime={minTime}
                    maxTime={maxTime}
                    PM={PM} />
            </div>
        </Form.Feedback>
    );
};

Form.TimeRange.components = function ({ name, context, styles = { width: '100%' } }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context?.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (!Array.isArray(context?.values?.[name]) || context?.values?.[name].length !== 2) {
        context.values[name] = [null, null];
    }

    return {
        Left: ({ minTime = null, maxTime = null, PM = false }) => (
            <Form.ClockTime
                name={name}
                context={{
                    ...context,
                    values: {
                        ...context.values,
                        [name]: context.values[name][0] || null,
                    },
                    setValues: (newValues) => {
                        context.setValues({ ...context.values, [name]: [newValues[name], context.values[name][1] || null] });
                    }
                }}
                minTime={minTime}
                maxTime={maxTime || context.values[name][1] || null}
                PM={PM}
                styles={styles}
            />
        ),
        Right: ({ minTime = null, maxTime = null, PM = false }) => (
            <Form.ClockTime
                name={name}
                context={{
                    ...context,
                    values: {
                        ...context.values,
                        [name]: context.values[name][1] || null,
                    },
                    setValues: (newValues) => {
                        context.setValues({ ...context.values, [name]: [context.values[name][0] || null, newValues[name]] });
                    }
                }}
                minTime={minTime || context.values[name][0] || null}
                maxTime={maxTime}
                PM={PM}
                styles={styles}
            />
        ),
    };
};

Form.Datetime = function ({ name, context, minDate = null, maxDate = null, minTime = null, maxTime = null, PM = false, styles = { width: '100%', maxWidth: '300px', overflowX: 'auto' } }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            <div
                className={`border rounded p-2 ${ruleset.isInvalid() ? 'border-danger' : ''}`}
                style={styles}
            >
                <Form.Calendar
                    name={name}
                    context={context}
                    ruleset={ruleset}
                    minDate={minDate}
                    maxDate={maxDate}
                >
                    <div className="border-top mt-2 pt-2">
                        <Form.Time
                            name={`${name}_time`}
                            context={context}
                            minTime={minTime}
                            maxTime={maxTime}
                            PM={PM}
                            styles={{ width: '100%', }} />
                    </div>
                </Form.Calendar>
            </div>
        </Form.Feedback>
    );
};

Form.DatetimeRange = function ({ name, context, minDate = null, maxDate = null, minTime = "00:00", maxTime = "23:59", PM = false, styles = { width: '100%', maxWidth: '768px', overflowX: 'auto' } }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    const date = Form.DateRange.components({ name, context, minDate, maxDate });
    const time = Form.TimeRange.components({ name: `${name}_time`, context });

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            <div className={`border rounded p-2 ${ruleset.isInvalid() ? 'border-danger' : ''}`} style={styles}>
                <div className="d-flex gap-2">
                    <div style={{ width: '50%' }}>
                        <date.Left >
                            <div className="border-top mt-2 pt-2">
                                <time.Left
                                    minTime={minTime}
                                    maxTime={maxTime}
                                    PM={PM} />
                            </div>
                        </date.Left>
                    </div>
                    <div className="border-start"></div>
                    <div style={{ width: '50%' }}>
                        <date.Right >
                            <div className="border-top mt-2 pt-2">
                                <time.Right
                                    minTime={minTime}
                                    maxTime={maxTime}
                                    PM={PM} />
                            </div>
                        </date.Right>
                    </div>
                </div>
            </div>
        </Form.Feedback>
    );
};

Form.Select = function ({ name, context, options = [], props = {} }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={false} arrowPositions={{ right: '39px' }}>
            <bs.Form.Select
                name={name}
                value={context.values[name] || ""}
                disabled={ruleset.isDisabled()}
                isInvalid={ruleset.isInvalid()}
                onChange={e => context.setValues({ ...context.values, [name]: e.target.value })}
                {...props}
            >
                {/* UN SAC D'ATTRIBUTS PAR OPTION, et non un prop par besoin.
                    `{ label, value, attributes: { disabled, ... } }` — le premier
                    usage est de griser une option indisponible sans griser le
                    sélecteur entier (un émulateur ou un BIOS que la source
                    choisie ne fournit pas doit rester VISIBLE, pour qu'on sache
                    qu'il existe). La forme est ouverte exprès : un futur
                    `data-*` ou `title` n'obligera pas à revenir toucher
                    skeleton. */}
                {options.map((option, i) => (
                    <option key={i} value={option.value} {...(option.attributes || {})}>
                        {option.label}
                    </option>
                ))}
            </bs.Form.Select>
        </Form.Feedback>
    );
};

Form.Longtext = function ({ name, context, props = { autoComplete: "off" } }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={false}>
            <bs.Form.Control
                as="textarea"
                name={name}
                value={context.values[name] || ""}
                disabled={ruleset.isDisabled()}
                isInvalid={ruleset.isInvalid()}
                onChange={e => context.setValues({ ...context.values, [name]: e.target.value })}
                onInput={(e) => {
                    e.target.style.height = 'auto';
                    e.target.style.height = (e.target.scrollHeight + 5) + 'px';
                }}
                {...props}
            />
        </Form.Feedback>
    );
};

Form.Checkbox = function ({ name, context, label = null, props = {} }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            <bs.Form.Label htmlFor={`checkbox-${name}`} className="d-flex align-items-center">
                <bs.Form.Check
                    id={`checkbox-${name}`}
                    type="checkbox"
                    name={name}
                    label={label}
                    checked={context.values[name] || false}
                    disabled={ruleset.isDisabled()}
                    isInvalid={ruleset.isInvalid()}
                    onChange={e => context.setValues({ ...context.values, [name]: e.target.checked })}
                    {...props}
                />
            </bs.Form.Label>
        </Form.Feedback>
    );
};

Form.Multicheckbox = function ({ name, context, options = [], props = {} }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = [];
    } else if (!Array.isArray(context.values[name])) {
        context.values[name] = [];
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            {(options || []).map((option, i) => (
                <bs.Form.Check
                    id={`checkbox-${name}-${i}`}
                    key={i}
                    type="checkbox"
                    name={name}
                    label={option.label}
                    checked={context.values[name] ? context.values[name].includes(option.value) : false}
                    disabled={ruleset.isDisabled()}
                    isInvalid={ruleset.isInvalid()}
                    onChange={e => {
                        const currentValues = context.values[name] || [];

                        if (e.target.checked) {
                            context.setValues({ ...context.values, [name]: [...currentValues, option.value] });
                        } else {
                            context.setValues({ ...context.values, [name]: currentValues.filter(v => v !== option.value) });
                        }
                    }}
                />
            ))}
        </Form.Feedback>
    );
};

Form.Radio = function ({ name, context, options = [], props = {} }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            {options.map((option, i) => (
                <bs.Form.Check
                    key={i}
                    type="radio"
                    id={`radio-${name}-${i}`}
                    name={name}
                    label={option.label}
                    value={option.value}
                    checked={context.values[name] === option.value}
                    disabled={ruleset.isDisabled()}
                    isInvalid={ruleset.isInvalid()}
                    onChange={e => context.setValues({ ...context.values, [name]: option.value })}
                />
            ))}
        </Form.Feedback>
    );
};

Form.SkeletonRadio = function ({ name, context, options = [], props = {} }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            <div className="d-flex flex-column gap-2">
                {(options || []).map(({ icon, title, subtitle }) => {
                    return (
                        <button
                            key={title}
                            type="button"
                            style={{
                                display: 'flex',
                                alignItems: 'center',
                                gap: '0.75rem',
                                width: '100%',
                                padding: '0.75rem 1rem',
                                background: context.values[name] === title ? 'var(--bs-primary-bg)' : 'var(--bs-body-bg)',
                                border: `1px solid ${ruleset.isInvalid() ? 'var(--bs-danger)' : context.values[name] === title ? 'var(--bs-primary)' : 'var(--bs-border-color)'}`,
                                borderRadius: 'var(--bs-border-radius)',
                                color: ruleset.isInvalid() ? 'var(--bs-danger)' : context.values[name] === title ? 'var(--bs-primary)' : 'var(--bs-body-color)',
                                textAlign: 'left',
                                cursor: ruleset.isDisabled() ? 'not-allowed' : 'pointer',
                                opacity: ruleset.isDisabled() ? 0.55 : 1,
                                transition: 'background-color 120ms, border-color 120ms',
                            }}
                            onClick={() => context.setValues({ ...context.values, [name]: title })}
                            disabled={ruleset.isDisabled()}
                        >
                            <span style={{
                                width: '2.5rem',
                                height: '2.5rem',
                                flexShrink: 0,
                                display: 'inline-flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                background: context.values[name] === title ? 'var(--bs-primary-bg)' : 'var(--bs-secondary-bg)',
                                borderRadius: 'var(--bs-border-radius)',
                                border: '1px solid var(--bs-border-color)',
                                fontSize: '1.25rem',
                            }}>
                                <i className={icon}></i>
                            </span>
                            <span style={{ flex: 1, minWidth: 0 }}>
                                <span style={{ fontWeight: 600, display: 'block' }}>{title}</span>
                                <span style={{ fontSize: '0.875rem', color: 'var(--bs-secondary-color)', display: 'block', marginTop: '2px' }}>{subtitle}</span>
                            </span>
                        </button>
                    );
                })}
            </div>
        </Form.Feedback>
    );
};

Form.SkeletonCheckbox = function ({ name, context, options = [], props = {} }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = [];
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            <div className="d-flex flex-column gap-2">
                {(options || []).map(({ icon, title, subtitle }) => {
                    return (
                        <button
                            key={title}
                            type="button"
                            style={{
                                display: 'flex',
                                alignItems: 'center',
                                gap: '0.75rem',
                                width: '100%',
                                padding: '0.75rem 1rem',
                                background: context.values[name]?.includes(title) ? 'var(--bs-primary-bg)' : 'var(--bs-body-bg)',
                                border: `1px solid ${ruleset.isInvalid() ? 'var(--bs-danger)' : context.values[name]?.includes(title) ? 'var(--bs-primary)' : 'var(--bs-border-color)'}`,
                                borderRadius: 'var(--bs-border-radius)',
                                color: ruleset.isInvalid() ? 'var(--bs-danger)' : context.values[name]?.includes(title) ? 'var(--bs-primary)' : 'var(--bs-body-color)',
                                textAlign: 'left',
                                cursor: ruleset.isDisabled() ? 'not-allowed' : 'pointer',
                                opacity: ruleset.isDisabled() ? 0.55 : 1,
                                transition: 'background-color 120ms, border-color 120ms',
                            }}
                            onClick={() => {
                                const currentValues = context.values[name] || [];
                                const newValues = currentValues.includes(title)
                                    ? currentValues.filter(v => v !== title)
                                    : [...currentValues, title];
                                context.setValues({ ...context.values, [name]: newValues });
                            }}
                            disabled={ruleset.isDisabled()}
                        >
                            <span style={{
                                width: '2.5rem',
                                height: '2.5rem',
                                flexShrink: 0,
                                display: 'inline-flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                background: context.values[name]?.includes(title) ? 'var(--bs-primary-bg)' : 'var(--bs-secondary-bg)',
                                borderRadius: 'var(--bs-border-radius)',
                                border: '1px solid var(--bs-border-color)',
                                fontSize: '1.25rem',
                            }}>
                                <i className={icon}></i>
                            </span>
                            <span style={{ flex: 1, minWidth: 0 }}>
                                <span style={{ fontWeight: 600, display: 'block' }}>{title}</span>
                                <span style={{ fontSize: '0.875rem', color: 'var(--bs-secondary-color)', display: 'block', marginTop: '2px' }}>{subtitle}</span>
                            </span>
                        </button>
                    );
                })}
            </div>
        </Form.Feedback>
    );
};

Form.Range = function ({ name, context, min = 0, max = 100, step = 1, variant = "md", props = {} }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    } else if (typeof context.values[name] === "string") {
        context.values[name] = Number(context.values[name]);
    } else if (typeof context.values[name] !== "number") {
        context.values[name] = null;
    }

    const progress = `${((Number(context.values[name] || min) - min) / (max - min)) * 100}%`;

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={false}>
            <div className={`form-range-group ${ruleset.isDisabled() ? "opacity-50" : ""}`}>
                <bs.Form.Range
                    name={name}
                    value={context.values[name] || min}
                    min={min}
                    max={max}
                    step={step}
                    disabled={ruleset.isDisabled()}
                    onChange={e => context.setValues({ ...context.values, [name]: e.target.value })}
                    style={{ '--range-progress': progress }}
                    {...props}
                />
                <bs.Form.Control
                    type="number"
                    value={context.values[name] || min}
                    min={min}
                    max={max}
                    step={step}
                    className={`form-range-value ${variant}`}
                    disabled={ruleset.isDisabled()}
                    isInvalid={ruleset.isInvalid()}
                    onChange={e => context.setValues({ ...context.values, [name]: e.target.value })}
                />
            </div>
        </Form.Feedback>
    );
};

Form.Switch = function ({ name, context, helpText = "", props = {} }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            <bs.Form.Label htmlFor={`switch-${name}`} className="d-flex align-items-center">
                <bs.Form.Check
                    id={`switch-${name}`}
                    type="switch"
                    name={name}
                    label={props.label || ""}
                    checked={context.values[name] || false}
                    disabled={ruleset.isDisabled()}
                    isInvalid={ruleset.isInvalid()}
                    onChange={e => context.setValues({ ...context.values, [name]: e.target.checked })}
                    {...props}
                />
                {helpText && (
                    <bs.Form.Text className="text-muted">{helpText}</bs.Form.Text>
                )}
            </bs.Form.Label>
        </Form.Feedback>
    );
};

Form.SkeletonSwitch = function ({ name, context, onText = "On", offText = "Off", label = null, helpText = null, reverse = false, variant = "sm", props = {} }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            <label
                className={`d-flex align-items-center gap-3 py-2 ${reverse ? "flex-row" : "flex-row-reverse"} ${ruleset.isDisabled() ? "opacity-50 pe-none" : ""}`}
                style={{ cursor: ruleset.isDisabled() ? 'not-allowed' : 'pointer' }}
            >
                <div style={{ flex: 1, minWidth: 0 }}>
                    {label && <span className={`d-block fw-semibold${ruleset.isInvalid() ? " text-danger" : ""}`}>{label}</span>}
                    {helpText && <span className="d-block small text-muted mt-1">{helpText}</span>}
                </div>
                <div className={`form-switch-pill ${variant}`}>
                    <input
                        type="checkbox"
                        checked={context.values[name] || false}
                        disabled={ruleset.isDisabled()}
                        onChange={e => context.setValues({ ...context.values, [name]: e.target.checked })}
                    />
                    <span className="switch-track" data-on={onText} data-off={offText}></span>
                </div>
            </label>
        </Form.Feedback>
    );
};

Form.SkeletonSwitchMode = function ({ name, context, options = [], label = null, props = {} }) {
    let ruleset = Ruleset.fallback();
    // Per-instance id prefix — the option ids used to be a bare
    // `option-${i}`, which is a DOCUMENT-wide identifier: two SwitchMode
    // rendered at once (a settings panel with several mode pickers, an
    // offcanvas over a page that already has one) both emitted `option-0`,
    // and every <label htmlFor> then resolved to whichever came first in
    // the DOM — clicking one instance toggled the other's.
    const instanceId = useId();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = null;
    }

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={true}>
            <div className="d-flex align-items-center gap-3 py-2">
                <div
                    className={`d-inline-flex p-1 gap-1 text-center ${ruleset.isDisabled() ? "opacity-50 pe-none" : ""}`}
                    style={{
                        width: '100%',
                        background: 'var(--bs-secondary-bg)',
                        border: '1px solid var(--bs-border-color)',
                        borderRadius: 'var(--bs-border-radius-pill)',
                    }}
                >
                    {options.map((option, i) => {
                        return (
                            <React.Fragment key={i}>
                                <input
                                    type="radio"
                                    id={`${instanceId}-option-${i}`}
                                    name={name}
                                    value={option.value}
                                    checked={context.values[name] === option.value}
                                    onChange={(e) => context.setValues({ ...context.values, [name]: e.target.value })}
                                    disabled={ruleset.isDisabled()}
                                    style={{
                                        position: 'absolute',
                                        opacity: 0,
                                        pointerEvents: 'none',
                                        width: 0,
                                        height: 0,
                                    }}
                                />
                                <label
                                    htmlFor={`${instanceId}-option-${i}`}
                                    className="d-inline-flex align-items-center text-center gap-2 px-3 py-1"
                                    style={{
                                        flex: 1,
                                        background: context.values[name] === option.value ? 'var(--bs-primary)' : 'transparent',
                                        borderRadius: 'var(--bs-border-radius-pill)',
                                        color: context.values[name] === option.value ? '#fff' : ruleset.isInvalid() ? 'var(--bs-danger)' : 'var(--bs-secondary-color)',
                                        cursor: ruleset.isDisabled() ? 'not-allowed' : 'pointer',
                                        fontWeight: 500,
                                        whiteSpace: 'nowrap',
                                        transition: 'background-color 200ms, color 200ms',
                                        userSelect: 'none',
                                    }}>
                                    <i className={option.icon}></i>
                                    {option.label}
                                </label>
                            </React.Fragment>
                        );
                    })}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                    {label && <span className={`d-block text-start ${ruleset.isInvalid() ? "text-danger" : ""}`}>{label}</span>}
                </div>
            </div>
        </Form.Feedback>
    );
};

Form.File = function ({ name, context, maxFiles = 1, maxFileSize = null, accept = null, props = {} }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.files, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.files?.[name] === undefined) {
        context.files[name] = [];
        context.values[name] = [];
    } else if (!Array.isArray(context.files[name])) {
        context.files[name] = [];
        context.values[name] = [];
    }

    maxFiles = maxFiles || 1;
    const inputRef = useRef(null);
    const files = useMemo(() => {
        if (Array.isArray(context.files[name])) {
            return context.files[name];
        }

        return context.files[name] || [];
    }, [context.files[name]]);
    const [previews, setPreviews] = useState([]);
    const [selectedFiles, setSelectedFiles] = useState(files);
    const processFiles = (selectedFiles) => {
        const validFiles = [];

        for (let i = 0; i < selectedFiles.length; i++) {
            const file = selectedFiles[i];

            if (maxFileSize && file.size > maxFileSize) {
                alert(`File "${file.name}" exceeds the maximum size of ${formatSize(maxFileSize)}.`);
                continue;
            }

            if (!validateAccept(file, accept)) {
                alert(`File "${file.name}" is not an accepted format.`);
                continue;
            }

            validFiles.push(file);

            // Le plafond atteint : l'alerte seulement s'il reste des fichiers
            // choisis en trop. Elle partait à chaque choix d'un fichier quand
            // maxFiles vaut 1 (relevé le 2026-10-26, chantier 33).
            if (validFiles.length + (maxFiles === 1 ? 0 : files.length) >= maxFiles) {
                if (i < selectedFiles.length - 1) {
                    alert(`You can only upload a maximum of ${maxFiles} file(s).`);
                }
                break;
            }
        }

        let newFiles = [];

        if (maxFiles === 1) {
            newFiles = validFiles.slice(0, 1);
        } else {
            newFiles = files.concat(validFiles).slice(0, maxFiles);
        }

        context.setFiles({ ...context.files, [name]: newFiles });
        context.setValues({ ...context.values, [name]: newFiles.map(file => ({ name: file.name, pValue: 0 })) });
    };
    const formatSize = (bytes) => {
        if (bytes === 0) {
            return '0 B';
        }

        if (!bytes) {
            return '-';
        }

        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));

        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    };
    const validateAccept = (file, accept) => {
        if (!accept) {
            return true;
        }

        const acceptedTypes = accept.split(',').map(t => t.trim());
        const fileName = file.name || '';
        const fileType = file.type || '';

        return acceptedTypes.some(type => {
            if (type.startsWith('.')) {
                // Extension check
                return fileName.toLowerCase().endsWith(type.toLowerCase());
            } else if (type.endsWith('/*')) {
                // Mime type group check (e.g. image/*)
                const mainType = type.split('/')[0];
                return fileType.startsWith(mainType + '/');
            } else {
                // Exact mime type check
                return fileType === type;
            }
        });
    };

    useEffect(() => {
        if (Array.isArray(files)) {
            const newPreviews = files.map(file => {
                if (file instanceof File && file.size > 0 && file.size <= 100 * 1024 * 1024) {
                    return URL.createObjectURL(file);
                }

                return null;
            });
            setPreviews(newPreviews);

            return () => {
                newPreviews.forEach(url => {
                    if (url && typeof url === 'string' && url.startsWith('blob:')) {
                        URL.revokeObjectURL(url);
                    }

                    return null;
                });
            };
        }
    }, [files]);

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={false}>
            <div
                className={ruleset.isDisabled() ? 'opacity-50 pe-none' : ''}
                style={ruleset.isInvalid() ? { outline: '1px solid var(--bs-danger)', borderRadius: 'var(--bs-border-radius)' } : {}}
            >
                <button
                    type="button"
                    className="text-center py-5 w-100 border btn btn-outline-secondary"
                    disabled={ruleset.isDisabled()}
                    onClick={() => !ruleset.isDisabled() && inputRef.current?.click()}
                >
                    <i className="bi bi-upload fs-1"></i>
                </button>
                {files.length > 0 && (
                    <div className="mt-3" style={{ maxHeight: '300px', overflowY: 'auto' }}>
                        {files.map((file, i) => {
                            const pValue = context.values[name] && context.values[name][i] ? context.values[name][i].pValue : 0;

                            return (
                                <div key={i} className="d-flex align-items-start justify-content-start gap-3 mb-2">
                                    {!ruleset.isDisabled() && (
                                        <button
                                            type="button"
                                            className="btn btn-ghost-danger"
                                            onClick={() => {
                                                const newFiles = files.filter((_, j) => j !== i);

                                                if (maxFiles === 1) {
                                                    context.setFiles({ ...context.files, [name]: newFiles[0] || null });
                                                    context.setValues({ ...context.values, [name]: newFiles[0] ? { name: newFiles[0].name, pValue: 0 } : null });
                                                } else {
                                                    context.setFiles({ ...context.files, [name]: newFiles });
                                                    context.setValues({ ...context.values, [name]: newFiles.map(file => ({ name: file.name, pValue: 0 })) });
                                                }

                                                setSelectedFiles(newFiles);
                                            }}
                                        >
                                            <i className="bi bi-trash"></i>
                                        </button>
                                    )}
                                    <div className="d-flex align-items-start justify-content-between w-100 gap-2">
                                        <div className="d-flex flex-column" style={{ maxWidth: 'calc(100% - 60px)' }}>
                                            <div>
                                                {((typeof file === 'string' || file.type) && previews[i]) ? (
                                                    <a
                                                        href={previews[i]}
                                                        target="_blank"
                                                        rel="noopener noreferrer"
                                                        className="text-decoration-none"
                                                    >
                                                        {file.name}
                                                    </a>
                                                ) : (
                                                    <span className="text-subtitle">{file.name}</span>
                                                )}
                                            </div>
                                            <div className="text-muted" style={{ fontSize: '0.9em' }}>{formatSize(file.size)}</div>
                                        </div>
                                        <div style={{
                                            width: '50px',
                                            height: '50px',
                                            flexShrink: 0,
                                            borderRadius: '50%',
                                            background: `conic-gradient(var(--bs-primary) ${pValue}%, var(--bs-secondary-bg) 0)`,
                                            display: 'inline-flex',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            fontSize: '0.70em',
                                            position: 'relative',
                                        }}>
                                            <div style={{
                                                position: 'absolute',
                                                inset: '8px',
                                                borderRadius: '50%',
                                                background: 'var(--bs-body-bg)',
                                            }}></div>
                                            <span style={{ position: 'relative', zIndex: 1 }}>{pValue}%</span>
                                        </div>
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                )}
                <bs.Form.Control
                    ref={inputRef}
                    type="file"
                    disabled={ruleset.isDisabled()}
                    isInvalid={ruleset.isInvalid()}
                    accept={accept}
                    multiple={(maxFiles || 1) > 1}
                    autoComplete="off"
                    readOnly={true}
                    style={{ display: 'none' }}
                    onChange={(e) => {
                        e.stopPropagation();
                        processFiles(e.target.files);
                    }}
                />
            </div>
        </Form.Feedback>
    );
};

Form.SkeletonList = function ({ name, context, inputType = "longtext", options = [], unique = true, props = {}, texts = {
    howTo: null,
    confirmDelete: null,
    confirmDeleteAll: null,
} }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = [];
    } else if (!Array.isArray(context.values[name])) {
        context.values[name] = [];
    }

    const value = Array.isArray(context.values[name]) ? context.values[name] : [];
    const [listItem, setListItem] = useState("");
    const [selectedItem, setSelectedItem] = useState(null);
    const [preSelectItem, setPreSelectItem] = useState(null);
    const [selectedItems, setSelectedItems] = useState(value);

    useEffect(() => {
        if (value.length === 0 && selectedItems.length > 0) {
            setSelectedItems([]);
        }
    }, [value]);

    // Clearing the text field while an item is selected removes that item —
    // no confirm() here (unlike the trash-icon handler below), since this is
    // an implicit side effect of editing, not an explicit delete action.
    const deleteItem = (index) => {
        const updatedItems = selectedItems.filter((_, j) => index !== j);

        setSelectedItems(updatedItems);
        context.setValues({ ...context.values, [name]: updatedItems });

        if (selectedItem === index) {
            setListItem("");
            setSelectedItem(null);
        }
    };

    return (
        <>
            {inputType === "longtext" ? (
                <Form.Longtext
                    name={name}
                    context={context}
                    props={{
                        ...props,
                        value: listItem,
                        onChange: (e) => {
                            if (selectedItem !== null && e.target.value === "") {
                                return deleteItem(selectedItem);
                            }

                            setListItem(e.target.value);

                            if (!selectedItems.includes(listItem.trim())) {
                                selectedItems[selectedItem] = e.target.value;
                                setSelectedItems([...selectedItems]);
                                context.setValues({ ...context.values, [name]: [...selectedItems] });
                            }

                        },
                    }}
                />
            ) : (
                <Form.Select
                    name={name}
                    context={context}
                    options={(options || []).map((option, i) => {
                        return (!selectedItems.includes(option.value)) ? { label: option.label, value: option.value } : null;
                    }).filter(i => i !== null)}
                    props={{
                        ...props,
                        value: listItem,
                        onChange: (e) => {
                            if (selectedItem !== null && e.target.value === "") {
                                return deleteItem(selectedItem);
                            }

                            setListItem(e.target.value);
                            selectedItems[selectedItem] = e.target.value;
                            setSelectedItems([...selectedItems]);
                            context.setValues({ ...context.values, [name]: [...selectedItems] });
                        },
                    }}
                />
            )}
            <div>
                <div className="d-flex justify-content-between mt-2">
                    {!ruleset.isDisabled() && selectedItems.length > 0 ? (
                        <button
                            type="button"
                            disabled={ruleset.isDisabled() || selectedItems.length === 0}
                            className="btn btn-ghost-danger py-1"
                            onClick={() => {
                                if (window.confirm(texts?.confirmDeleteAll || "Are you sure you want to delete all items? This action cannot be undone!")) {
                                    setSelectedItems([]);
                                    context.setValues({ ...context.values, [name]: [] });
                                    setListItem("");
                                    setSelectedItem(null);
                                }
                            }}
                        >
                            <i className="bi bi-trash-fill"></i>
                        </button>
                    ) : <div />}
                    <button
                        type="button"
                        disabled={ruleset.isDisabled()}
                        className={`btn ${selectedItem !== null ? "btn-secondary" : "btn-primary"} px-4 py-1`}
                        onClick={() => {
                            if (listItem && listItem.trim() !== "") {
                                if (!unique || !selectedItems.includes(listItem.trim())) {
                                    setSelectedItems([...selectedItems, listItem.trim()]);
                                    context.setValues({ ...context.values, [name]: [...selectedItems, listItem.trim()] });
                                    setListItem("");
                                    setSelectedItem(null);
                                } else {
                                    setListItem("");
                                    setSelectedItem(null);
                                }
                            } else if (selectedItem) {
                                setListItem("");
                                setSelectedItem(null);
                            }
                        }}
                    >
                        <i className={selectedItem !== null ? "bi bi-three-dots" : "bi bi-plus"}></i>
                    </button>
                </div>
                {selectedItems.length > 0 && (
                    <ul key="selected-items-list" className="mt-3 mb-0 ps-3" style={{ listStyleType: "disc", maxHeight: '200px', overflowY: 'auto' }}>
                        {selectedItems.map((item, i) => (
                            <li
                                key={i}
                                onMouseEnter={() => setPreSelectItem(i)}
                                onMouseLeave={() => setPreSelectItem(null)}
                            >
                                <div className="d-flex align-items-center justify-content-between">
                                    <span
                                        style={{
                                            flex: 1,
                                            cursor: ruleset.isDisabled() ? 'default' : 'pointer',
                                            fontWeight: selectedItem === i ? 'bold' : 'normal',
                                            textDecoration: preSelectItem === i ? 'underline' : 'none',
                                        }}
                                        onClick={() => {
                                            if (!ruleset.isDisabled()) {
                                                setListItem(item);
                                                setSelectedItem(i);
                                            }
                                        }}
                                    >
                                        {item}
                                    </span>
                                    {!ruleset.isDisabled() && preSelectItem === i && (
                                        <span
                                            className="text-danger"
                                            style={{ cursor: 'pointer' }}
                                            onClick={() => {
                                                if (window.confirm(texts?.confirmDelete || "Are you sure you want to delete this item? This action cannot be undone!")) {
                                                    const updatedItems = selectedItems.filter((_, j) => i !== j);

                                                    setSelectedItems(updatedItems);
                                                    context.setValues({ ...context.values, [name]: updatedItems });

                                                    if (selectedItem === i) {
                                                        setListItem("");
                                                        setSelectedItem(null);
                                                    }
                                                }
                                            }}
                                        >
                                            <i className="bi bi-trash"></i>
                                        </span>
                                    )}
                                </div>
                            </li>
                        ))}
                    </ul>
                )}
            </div>
        </>
    );
};

Form.SkeletonSmallForm = function ({ name, context, fields = [], maxItems = null, texts = {
    confirmDelete: null,
    confirmDeleteAll: null,
} }) {
    let ruleset = Ruleset.fallback();

    if (context?.rulesets?.[name]) {
        ruleset = Ruleset(context.rulesets[name], context.values, name);
    }

    if (ruleset.isHidden()) {
        return null;
    }

    if (context?.values?.[name] === undefined) {
        context.values[name] = [];
    } else if (!Array.isArray(context.values[name])) {
        context.values[name] = [];
    }

    const maxItemReached = maxItems !== null && context?.values?.[name] && context.values[name].length >= maxItems;
    const appendItem = () => {
        if (!maxItemReached) {
            context.setValues({ ...context.values, [name]: [...(context.values[name] || []), {}] });
        }
    };

    const deleteItems = (i = null) => {
        if (i === null) {
            if (window.confirm(texts?.confirmDeleteAll || "Are you sure you want to delete all items? This action cannot be undone!")) {
                context.setValues({ ...context.values, [name]: [] });
            }
        } else {
            let status = true;

            if (texts?.confirmDelete || false) {
                status = window.confirm(texts.confirmDelete);
            }

            if (status) {
                context.setValues({ ...context.values, [name]: context.values[name].filter((_, j) => i !== j) });
            }
        }
    };

    return (
        <Form.Feedback message={ruleset.message} isFeedbackOnLeft={false}>
            <div
                className={ruleset.isDisabled() ? "opacity-50 pe-none" : ""}
                style={ruleset.isInvalid() ? { outline: '1px solid var(--bs-danger)', borderRadius: 'var(--bs-border-radius)' } : {}}
            >
                {(context?.values?.[name] || []).length === 0 ? (
                    <button
                        type="button"
                        className="text-center py-5 w-100 border btn btn-outline-secondary"
                        onClick={appendItem}>
                        <i className="bi bi-pencil fs-1"></i>
                    </button>
                ) : <React.Fragment>
                    {(context?.values?.[name] || []).map((value, i) => {
                        const itemRuleset = context?.rulesets?.[`${name}.nested`] || {};
                        const itemContext = {
                            values: value,
                            setValues: (newValue) => {
                                const updatedValues = [...context?.values?.[name]];
                                updatedValues[i] = newValue;
                                context.setValues({ ...context.values, [name]: updatedValues });
                            },
                            rulesets: (typeof itemRuleset === "function") ? itemRuleset(i) : null,
                            i: i,
                        };

                        return (
                            <div key={i} className="d-flex justify-content-start gap-3">
                                <button
                                    type="button"
                                    className="btn btn-ghost-danger align-self-start"
                                    onClick={() => deleteItems(i)}
                                >
                                    <i className="bi bi-trash"></i>
                                </button>
                                <div className="flex-grow-1 border-bottom mb-4">
                                    {fields.map((field, j) => {
                                        if (field.component !== Form.Group) {
                                            return React.createElement(field.component, {
                                                key: j,
                                                name: name,
                                                context: itemContext,
                                            });
                                        }

                                        return (React.createElement(field.component, {
                                            ...field.props,
                                            key: j,
                                            children: (field.props?.children || []).map((child, k) => {
                                                return React.createElement(child.component, {
                                                    ...child.props,
                                                    key: k,
                                                    context: itemContext,
                                                });
                                            }),
                                        }));
                                    })}
                                </div>
                            </div>
                        );
                    })}
                    <div key="last-line" className="d-flex justify-content-start gap-3">
                        <button
                            type="button"
                            className="btn btn-ghost-danger align-self-start"
                            onClick={() => deleteItems()}
                        >
                            <i className="bi bi-trash-fill"></i>
                        </button>
                        <button
                            type="button"
                            className="btn btn-primary w-100"
                            disabled={maxItemReached}
                            onClick={() => appendItem()}
                        >
                            {maxItemReached ? <i className="bi bi-exclamation-circle"></i> : <i className="bi bi-plus"></i>}
                        </button>
                    </div>
                </React.Fragment>}
            </div>
        </Form.Feedback>
    );
};