import React, { useState, useContext } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../skeleton.js";


const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

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

export default function FormDoc() {
	const context_ = useContext(Skeleton.context);

	const [values, setValues] = useState({});
	const [files, setFiles] = useState([]);
	const rulesets = {};

	rulesets["text"] = (data) => {
		if (data.intFloat < 18) {
			return Ruleset.isDisabled("Text input is disabled if the number is less than 18");
		}

		if (!data.text || data.text.length < 3) {
			return Ruleset.isInvalid("Text must be at least 3 characters");
		}

		if (!data.text || data.text.length > 30) {
			return Ruleset.isInvalid("Text must be at most 30 characters");
		}

		return null;
	};

	rulesets["password_strength"] = (data) => {
		if (!data.password || data.passwordStrength < 3) {
			return Ruleset.isInvalid("Password strength must be at least 3 (out of 5)");
		}

		return null;
	};

	// We create the ruleset instances here to be able to use them within each other's validation logic
	let dateRuleset = Ruleset.fallback();

	rulesets["date"] = (data) => {
		if (!data.date) {
			return Ruleset.isInvalid("Please select a date");
		}

		if (new Date(data.date) < new Date("2000-05-11") || new Date(data.date) > new Date("2016-03-21")) {
			return Ruleset.isInvalid(`Selected date ${data.date} is out of range. Please select a date between May 11, 2000 and March 21, 2016`);
		}

		return null;
	};

	// We initialize the ruleset instances here to be able to use them within each other's validation logic
	dateRuleset = Ruleset(rulesets["date"], values, "date");
	rulesets["dateRange"] = (data) => {
		if (!data.dateRange || data.dateRange.length !== 2 || !data.dateRange[0] || !data.dateRange[1]) {
			return Ruleset.isInvalid("Please select a date range");
		}

		let countDays = (new Date(data.dateRange[1]) - new Date(data.dateRange[0])) / (1000 * 60 * 60 * 24);

		if (countDays < 5) {
			return Ruleset.isInvalid("Please select a date range of at least 5 days");
		}

		return null;
	};
	rulesets["skeletonRadio"] = (data) => {
		if (!data.skeletonRadio) {
			return Ruleset.isInvalid("Please select a radio option");
		}

		return null;
	};
	rulesets["skeletonCheckbox"] = (data) => {
		if (!data.skeletonCheckbox || data.skeletonCheckbox.length < 2) {
			return Ruleset.isInvalid("Please select at least two checkbox options");
		}

		return null;
	};
	rulesets["skeletonFile"] = (data) => {
		if (!data.skeletonFile || data.skeletonFile.length === 0) {
			return Ruleset.isInvalid("Please upload at least one file");
		}

		return null;
	};
	rulesets["skeletonSwitchMode"] = (data) => {
		if (!data.skeletonSwitchMode || data.skeletonSwitchMode === "") {
			return Ruleset.isInvalid("Please select a switch mode option");
		}

		return null;
	};
	rulesets["skeletonSwitch"] = (data) => {
		if (!data.skeletonSwitch) {
			return Ruleset.isInvalid("Please select a switch option");
		}

		return null;
	};
	rulesets["skeletonRange"] = (data) => {
		if (!data.skeletonRange || data.skeletonRange <= 0 || data.skeletonRange > 999) {
			return Ruleset.isInvalid("Please select a value between 0 and 999");
		}

		return null;
	};
	rulesets["skeletonList"] = (data) => {
		if (!data.skeletonList || data.skeletonList.length === 0) {
			return Ruleset.isInvalid("Please add at least one item to the list");
		}

		return null;
	};
	rulesets["skeletonSmallForm"] = (data) => {
		if (!data.skeletonSmallForm || data.skeletonSmallForm.length === 0) {
			return Ruleset.isInvalid("Please add at least one SkeletonSmallForm item");
		} else if (data.skeletonSmallForm && data.skeletonSmallForm.length > 3) {
			return Ruleset.isInvalid("Maximum SkeletonSmallForm reached (3 items max)");
		}

		return null;
	};
	rulesets["skeletonSmallForm.nested"] = (i) => {
		// key name must ends with .nested to work.
		// The callback will receive the item index as parameter (starting from 0) and should return an object with the rulesets for that specific item
		return {
			hide: (data) => i == 1 ? Ruleset.isHidden() : null, // hide the second item (index 1) for demonstration purposes
			contact: (data) => {
				if (!data.contact || data.contact.length < 3) {
					return Ruleset.isInvalid("Skeleton Small Form contact must be at least 3 characters");
				}

				return null;
			},
			contactDate: (data) => {
				if (i != 1 && !data.contactDate) {
					return Ruleset.isInvalid("Please select a date");
				}

				return null;
			},
			contactTime: (data) => {
				if (i != 1 && !data.contactTime) {
					return Ruleset.isInvalid("Please select a time");
				}

				return null;
			},
		};
	};

	const context = { values, setValues, files, setFiles, rulesets };
	const CustomSkeletonSmallFormComponent = ({ name, context }) => {
		return (
			<>{context.rulesets.hide(context.i) ? (
				<p className="text-muted">No more details can be added for this item.</p>
			) : (
				<>
					<p>{context.i}) Illustration of how to implement custom components within a Skeleton Small Form</p>
					<div className="d-flex justify-content-between gap-2 p-3 rounded skeleton-surface-2">
						<div className="w-100">
							<Form.Group label="Date" styles={{ marginBottom: '0' }}>
								<Form.Date
									name="contactDate"
									context={context}
								/>
							</Form.Group>
						</div>
						<div className="w-100">
							<Form.Group label="Time" styles={{ marginBottom: '0' }}>
								<Form.Time
									name="contactTime"
									context={context} />
							</Form.Group>
						</div>
					</div>
				</>
			)}</>
		);
	};
	const searchTerms = [
		"Apple", "Apricot", "Avocado", "Banana", "Blueberry", "Cantaloupe", "Cherry",
		"Clementine", "Date", "Dragonfruit", "Durian", "Elderberry", "Fig", "Grape",
		"Grapefruit", "Guava",
	];

	const [isPending, setPending] = useState(false);
	const InputResult = ({ name = [] }) => {
		let toJson = {};

		name.forEach(n => {
			if (context.files[n] !== undefined) {
				toJson[n] = { "Files": context.files[n].map(f => ({ name: f.name, size: f.size, type: f.type })) };
			} else {
				toJson[n] = context.values[n] || null;
			}
		});

		return (
			<pre className="bg-body-tertiary border mb-4 p-2 rounded"><code style={{ fontSize: "0.8rem" }}>{JSON.stringify(toJson, null, 4)}</code></pre>
		);
	};

	return (
		<div className="container">
			<div className="mb-5">
				<h1 className="display-5 fw-bold mb-3">Form</h1>
				<p className="lead text-muted">
					Form components with built-in validation (<code>Ruleset</code>) and submission (<code>Form.submit</code>) — every field below is live, backed by the same shared <code>context</code>.
				</p>
			</div>

			<Form>
				{/* Section 1: Text inputs */}
				<div className="mb-5">
					<h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">1</bs.Badge>Text inputs</h2>
					<p><code>Form.SearchText</code> (async suggestions), <code>Form.Text</code>, <code>Form.Password</code> (with strength meter), <code>Form.Longtext</code>.</p>

					<div className="mb-3">
						<h6 className="text-muted mb-3">RESULT</h6>
						<Form.Group label="Search Text" helpText="Please enter your search text">
							<Form.SearchText
								name="searchText"
								callback={(term) => term ? searchTerms.filter(t => t.toLowerCase().includes(term.toLowerCase())).map(t => ({ value: t, label: `Fruit: ${t}` })) : []}
								context={context} />
						</Form.Group>
						<InputResult name={["searchText"]} />

						<Form.Group label="Text" helpText="Please enter your text">
							<Form.Text name="text" context={context} />
						</Form.Group>
						<InputResult name={["text"]} />

						<Form.Group label="Password" helpText="Please enter your password">
							<Form.Password name="password" context={context} withStrengthEvaluation={true} />
						</Form.Group>
						<InputResult name={["password", "password_strength"]} />

						<Form.Group label="Long Text (Textarea)" helpText="Please enter a longer text">
							<Form.Longtext name="longtext" context={context} props={{ rows: 3 }} />
						</Form.Group>
						<InputResult name={["longtext"]} />
					</div>

					<CodeBlock>{`<Form.Group label="Search Text">
    <Form.SearchText name="searchText" context={context}
        callback={(term) => term ? mySearch(term) : []} />
</Form.Group>
<Form.Group label="Text"><Form.Text name="text" context={context} /></Form.Group>
<Form.Group label="Password">
    <Form.Password name="password" context={context} withStrengthEvaluation={true} />
</Form.Group>
<Form.Group label="Long Text"><Form.Longtext name="longtext" context={context} props={{ rows: 3 }} /></Form.Group>`}</CodeBlock>
				</div>

				{/* Section 2: Numbers */}
				<div className="mb-5">
					<h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">2</bs.Badge>Numbers</h2>
					<p><code>Form.IntFloat</code> — bounded numeric input with <code>min</code>/<code>max</code>/<code>step</code>.</p>

					<div className="mb-3">
						<h6 className="text-muted mb-3">RESULT</h6>
						<Form.Group label="Int/Float" helpText="Please enter a number/float">
							<Form.IntFloat name="intFloat" context={context} min={18} max={50} step={1} />
						</Form.Group>
						<InputResult name={["intFloat"]} />
					</div>

					<CodeBlock>{`<Form.Group label="Int/Float">
    <Form.IntFloat name="intFloat" context={context} min={18} max={50} step={1} />
</Form.Group>`}</CodeBlock>
				</div>

				{/* Section 3: Date & Time */}
				<div className="mb-5">
					<h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">3</bs.Badge>Date & Time</h2>
					<p><code>Form.Date</code>, <code>Form.Datetime</code>, <code>Form.DateRange</code>, <code>Form.DatetimeRange</code>, <code>Form.Time</code>, <code>Form.TimeRange</code> — all hand-rolled (no native <code>&lt;input type="date"&gt;</code>).</p>

					<div className="mb-3">
						<h6 className="text-muted mb-3">RESULT</h6>
						<Form.Group label="Date" helpText="Please enter your date">
							<Form.Date name="date" context={context} />
						</Form.Group>
						<InputResult name={["date"]} />

						<Form.Group label="Datetime" helpText="Please enter your datetime">
							<Form.Datetime name="datetime" context={context} />
						</Form.Group>
						<InputResult name={["datetime", "datetime_time"]} />

						<Form.Group label="Date Range" helpText="Please enter your date range">
							<Form.DateRange name="dateRange" minDate="2026-05-11" maxDate="2027-06-20" context={context} />
						</Form.Group>
						<InputResult name={["dateRange"]} />

						<Form.Group label="Datetime Range" helpText="Please enter your datetime range">
							<Form.DatetimeRange name="datetimeRange" context={context} />
						</Form.Group>
						<InputResult name={["datetimeRange", "datetimeRange_time"]} />

						<Form.Group label="Time" helpText="Please enter your time">
							<Form.Time name="time" context={context} />
						</Form.Group>
						<InputResult name={["time"]} />

						<Form.Group label="Time Range" helpText="Please enter your time range">
							<Form.TimeRange name="timeRange" context={context} />
						</Form.Group>
						<InputResult name={["timeRange"]} />
					</div>

					<CodeBlock>{`<Form.Date name="date" context={context} />
<Form.Datetime name="datetime" context={context} />
<Form.DateRange name="dateRange" minDate="2026-05-11" maxDate="2027-06-20" context={context} />
<Form.DatetimeRange name="datetimeRange" context={context} />
<Form.Time name="time" context={context} />
<Form.TimeRange name="timeRange" context={context} />`}</CodeBlock>
				</div>

				{/* Section 4: Selection */}
				<div className="mb-5">
					<h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">4</bs.Badge>Selection</h2>
					<p><code>Form.Select</code>, <code>Form.Checkbox</code>, <code>Form.Multicheckbox</code>, <code>Form.Radio</code>, <code>Form.Switch</code> — thin wrappers over native Bootstrap form controls.</p>

					<div className="mb-3">
						<h6 className="text-muted mb-3">RESULT</h6>
						<Form.Group label="Select" helpText="Please choose an option">
							<Form.Select name="select" context={context} options={[
								{ value: "", label: "Select an option" },
								{ value: "red", label: "Red" },
								{ value: "green", label: "Green" },
								{ value: "blue", label: "Blue" },
							]} />
						</Form.Group>
						<InputResult name={["select"]} />

						<Form.Group label="Checkbox" helpText="Please check the box">
							<Form.Checkbox name="checkbox" context={context} label="Some text for the checkbox" />
						</Form.Group>
						<InputResult name={["checkbox"]} />

						<Form.Group label="Multiple choice (Multicheckbox)" helpText="Select one or more options you like">
							<Form.Multicheckbox name="multicheckbox" context={context} options={[
								{ value: "apple", label: "Apple" },
								{ value: "banana", label: "Banana" },
								{ value: "cherry", label: "Cherry" },
							]} />
						</Form.Group>
						<InputResult name={["multicheckbox"]} />

						<Form.Group label="Radio buttons" helpText="Select one option">
							<Form.Radio name="radio" context={context} options={[
								{ value: "male", label: "Male" },
								{ value: "female", label: "Female" },
								{ value: "other", label: "Other" },
							]} />
						</Form.Group>
						<InputResult name={["radio"]} />

						<Form.Group label="Switches">
							<Form.Switch name="switch1" context={context} label="A simple switch" helpText="Toggle this switch on or off." />
							<Form.Switch name="switch2" context={context} label="A second switch" helpText="Toggle this switch on or off." />
						</Form.Group>
						<InputResult name={["switch1", "switch2"]} />
					</div>

					<CodeBlock>{`<Form.Select name="select" context={context} options={[{ value: "red", label: "Red" }]} />
<Form.Checkbox name="checkbox" context={context} label="Some text" />
<Form.Multicheckbox name="multicheckbox" context={context} options={[{ value: "apple", label: "Apple" }]} />
<Form.Radio name="radio" context={context} options={[{ value: "male", label: "Male" }]} />
<Form.Switch name="switch1" context={context} label="A simple switch" />`}</CodeBlock>
				</div>

				{/* Section 5: Skeleton custom components */}
				<div className="mb-5">
					<h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">5</bs.Badge>Skeleton custom components</h2>
					<p>Card-style inputs specific to this template — <code>Form.Range</code> (filled track), <code>Form.SkeletonSwitch</code> (pill), <code>Form.SkeletonRadio</code>/<code>SkeletonCheckbox</code> (icon cards), <code>Form.SkeletonSwitchMode</code>, <code>Form.SkeletonList</code>.</p>

					<div className="mb-3">
						<h6 className="text-muted mb-3">RESULT</h6>
						<Form.Group label="Skeleton Range" helpText="A range component with a filled track.">
							<Form.Range name="skeletonRange" context={context} min={0} max={1000} step={10} />
						</Form.Group>
						<InputResult name={["skeletonRange"]} />

						<Form.Group label="Skeleton Switch" helpText="A pill-style switch.">
							<Form.SkeletonSwitch name="skeletonSwitch" context={context} label="Skeleton Switch" />
							<Form.SkeletonSwitch name="skeletonSwitch2" context={context} reverse={true} label="Skeleton Switch (reversed)" />
						</Form.Group>
						<InputResult name={["skeletonSwitch", "skeletonSwitch2"]} />

						<Form.Group label="Skeleton Radio" helpText="Icon-card radio options.">
							<Form.SkeletonRadio name="skeletonRadio" context={context} options={[
								{ icon: "bi bi-laptop", title: "Laptop", subtitle: "This is the first option" },
								{ icon: "bi bi-phone", title: "Phone", subtitle: "This is the second option" },
								{ icon: "bi bi-tv", title: "TV", subtitle: "This is the third option" },
							]} />
						</Form.Group>
						<InputResult name={["skeletonRadio"]} />

						<Form.Group label="Skeleton Checkbox" helpText="Icon-card checkbox options.">
							<Form.SkeletonCheckbox name="skeletonCheckbox" context={context} options={[
								{ icon: "bi bi-apple", title: "Apple", subtitle: "This is the first option" },
								{ icon: "bi bi-amd", title: "AMD", subtitle: "This is the second option" },
								{ icon: "bi bi-amazon", title: "Amazon", subtitle: "This is the third option" },
								{ icon: "bi bi-bing", title: "Bing", subtitle: "This is the fourth option" },
							]} />
						</Form.Group>
						<InputResult name={["skeletonCheckbox"]} />

						<Form.Group label="Skeleton Switch Mode" helpText="Segmented single-choice switch.">
							<Form.SkeletonSwitchMode name="skeletonSwitchMode" context={context} options={[
								{ icon: "bi bi-sun", label: "Light", value: "light" },
								{ icon: "bi bi-moon", label: "Dark", value: "dark" },
								{ icon: "bi bi-laptop", label: "System", value: "system" },
							]} />
						</Form.Group>
						<InputResult name={["skeletonSwitchMode"]} />

						<Form.Group label="Skeleton List" helpText="Repeatable single-value list.">
							<Form.SkeletonList name="skeletonList" inputType="select" context={context} options={[
								{ value: "", label: "---" },
								{ value: "Harry Potter and the Sorcerer's Stone", label: "Harry Potter and the Sorcerer's Stone" },
								{ value: "The Lord of the Rings: The Fellowship of the Ring", label: "The Lord of the Rings: The Fellowship of the Ring" },
								{ value: "To Kill a Mockingbird", label: "To Kill a Mockingbird" },
							]} />
						</Form.Group>
						<InputResult name={["skeletonList"]} />
					</div>

					<CodeBlock>{`<Form.Range name="skeletonRange" context={context} min={0} max={1000} step={10} />
<Form.SkeletonSwitch name="skeletonSwitch" context={context} label="Skeleton Switch" />
<Form.SkeletonRadio name="skeletonRadio" context={context} options={[{ icon: "bi bi-laptop", title: "Laptop", subtitle: "..." }]} />
<Form.SkeletonCheckbox name="skeletonCheckbox" context={context} options={[{ icon: "bi bi-apple", title: "Apple", subtitle: "..." }]} />
<Form.SkeletonSwitchMode name="skeletonSwitchMode" context={context} options={[{ icon: "bi bi-sun", label: "Light", value: "light" }]} />
<Form.SkeletonList name="skeletonList" inputType="select" context={context} options={[{ value: "x", label: "X" }]} />`}</CodeBlock>
				</div>

				{/* Section 6: File upload */}
				<div className="mb-5">
					<h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">6</bs.Badge>File upload</h2>
					<p><code>Form.File</code> — drag & drop, multi-file, with previews and per-file progress tracked in <code>context.values[name]</code> once submitted via <code>Form.submit</code>.</p>

					<div className="mb-3">
						<h6 className="text-muted mb-3">RESULT</h6>
						<Form.Group label="Skeleton File" helpText="Upload up to 5 files.">
							<Form.File name="skeletonFile" context={context} maxFiles={5} />
						</Form.Group>
						<InputResult name={["skeletonFile"]} />
					</div>

					<CodeBlock>{`<Form.Group label="Skeleton File">
    <Form.File name="skeletonFile" context={context} maxFiles={5} />
</Form.Group>`}</CodeBlock>
				</div>

				{/* Section 7: Repeatable groups */}
				<div className="mb-5">
					<h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">7</bs.Badge>Repeatable groups</h2>
					<p><code>Form.SkeletonSmallForm</code> — a repeatable array of field groups, mixing built-in fields with a fully custom component (per-item rulesets via the <code>"name.nested"</code> convention).</p>

					<div className="mb-3">
						<h6 className="text-muted mb-3">RESULT</h6>
						<Form.Group label="Skeleton Small Form" helpText="Add up to 3 items.">
							<Form.SkeletonSmallForm
								name="skeletonSmallForm"
								context={context}
								maxItems={3}
								fields={[
									{
										component: Form.Group,
										props: {
											helpText: "Please enter the item details",
											styles: { marginBottom: '1rem' },
											children: [
												{ component: Form.Text, props: { name: "contact", props: { placeholder: "Contact name" } } }
											]
										},
									},
									{ component: CustomSkeletonSmallFormComponent, props: { name: "customComponent", context } }
								]}
							/>
						</Form.Group>
						<InputResult name={["skeletonSmallForm"]} />
					</div>

					<CodeBlock>{`// rulesets["skeletonSmallForm.nested"] = (i) => ({
//     contact: (data) => (!data.contact || data.contact.length < 3)
//         ? Ruleset.isInvalid("At least 3 characters") : null,
// });
<Form.SkeletonSmallForm name="skeletonSmallForm" context={context} maxItems={3} fields={[
    { component: Form.Group, props: { children: [{ component: Form.Text, props: { name: "contact" } }] } },
    { component: CustomSkeletonSmallFormComponent, props: { name: "customComponent", context } },
]} />`}</CodeBlock>
				</div>

				{/* Section 8: Validation */}
				<div className="mb-5">
					<h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">8</bs.Badge>Validation with Ruleset</h2>
					<p>
						Each field can be tied to a rule in <code>context.rulesets</code> returning <code>Ruleset.isInvalid(message)</code>, <code>Ruleset.isDisabled(message)</code>, <code>Ruleset.isHidden()</code>, or <code>null</code> when valid.
						<code>Ruleset.hasFeedbacks(context)</code> walks every ruleset (including nested array rulesets) and returns <code>null</code> once the whole form is valid.
					</p>

					<div className="mb-3">
						<h6 className="text-muted mb-3">RESULT — live feedback state</h6>
						<pre className="bg-body-tertiary border mb-0 p-2 rounded"><code style={{ fontSize: "0.8rem" }}>{JSON.stringify(Ruleset.hasFeedbacks(context), null, 4)}</code></pre>
					</div>

					<CodeBlock>{`rulesets["text"] = (data) => {
    if (!data.text || data.text.length < 3) {
        return Ruleset.isInvalid("Text must be at least 3 characters");
    }
    return null;
};
const isFormValid = Ruleset.hasFeedbacks(context) === null;`}</CodeBlock>
				</div>

				{/* Section 9: Submitting */}
				<div className="mb-5">
					<h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="primary" className="me-2">9</bs.Badge>Submitting the form</h2>
					<p>
						<code>Form.submit(url, context, options)</code> posts <code>context.values</code> once, then uploads every file in <code>context.files</code> in parallel (one request each, so each gets its own progress event) — see <code>context.values[name][i].pValue</code> update live while uploading.
					</p>

					<div className="mb-3">
						<h6 className="text-muted mb-3">RESULT</h6>
						<div className="d-flex justify-content-end gap-3">
							<bs.Button variant="outline-secondary" onClick={() => setValues({})}>Reset</bs.Button>
							<bs.Button
								variant="primary"
								className="px-5"
								disabled={isPending}
								onClick={() => {
									setPending(true);
									// This demo posts to the shipped example REST endpoint just to show a real round trip —
									// point this at your own model's endpoint (see DeclarativeBase.REST() server-side).
									Form.submit("/api/consoles", context, { headers: {}, method: "POST" })
										.then(({ responses }) => {
											setPending(false);
											context_.toast.open(`Server responded (${responses.length} response(s)) — see console for details`, { variant: "info", title: "Form submitted" });
											console.log("Response from server:", responses);
										})
										.catch(error => {
											setPending(false);
											context_.toast.open("Submit failed — see console for details", { variant: "error", title: "Error" });
											console.error("Error submitting form:", error);
										});
								}}
							>
								{isPending ? <i className="fa-solid fa-circle-notch fa-spin"></i> : "Submit"}
							</bs.Button>
						</div>
					</div>

					<CodeBlock>{`Form.submit("/api/your-endpoint", context, { headers: {}, method: "POST" })
    .then(({ responses }) => context.toast.open("Submitted", { variant: "success" }))
    .catch(error => context.toast.open("Submit failed", { variant: "error" }));`}</CodeBlock>
				</div>
			</Form>

			{/* API Reference */}
			<div className="mb-5">
				<h2 className="border-bottom pb-2 mb-3"><bs.Badge bg="secondary" className="me-2">API</bs.Badge>API Reference</h2>

				<h6 className="mt-4 mb-2">Shared <code>context</code> shape</h6>
				<div className="table-responsive mb-4">
					<table className="table table-bordered">
						<thead>
							<tr><th style={{ width: "30%" }}>Key</th><th>Description</th></tr>
						</thead>
						<tbody>
							<tr><td><code>values</code> / <code>setValues</code></td><td>Field values (primitives, arrays, JSON objects).</td></tr>
							<tr><td><code>files</code> / <code>setFiles</code></td><td>Real <code>File</code> objects for upload fields only.</td></tr>
							<tr><td><code>rulesets</code></td><td>Map of field name → validation function.</td></tr>
						</tbody>
					</table>
				</div>

				<h6 className="mt-4 mb-2"><code>Ruleset</code></h6>
				<div className="table-responsive mb-4">
					<table className="table table-bordered">
						<thead>
							<tr><th style={{ width: "30%" }}>Method</th><th>Description</th></tr>
						</thead>
						<tbody>
							<tr><td><code>Ruleset.isInvalid(message)</code></td><td>Marks the field invalid with a feedback message.</td></tr>
							<tr><td><code>Ruleset.isDisabled(message)</code></td><td>Disables the field.</td></tr>
							<tr><td><code>Ruleset.isHidden()</code></td><td>Hides the field (and its <code>Form.Group</code>).</td></tr>
							<tr><td><code>Ruleset.hasFeedbacks(context)</code></td><td>Returns <code>null</code> if every ruleset passes, otherwise the first feedback found.</td></tr>
						</tbody>
					</table>
				</div>

				<h6 className="mt-4 mb-2"><code>Form.submit(url, context, options, submitId?)</code></h6>
				<div className="table-responsive">
					<table className="table table-bordered">
						<thead>
							<tr><th style={{ width: "30%" }}>Parameter</th><th>Description</th></tr>
						</thead>
						<tbody>
							<tr><td><code>url</code></td><td>Endpoint receiving the values POST; each file is posted to the same URL with <code>?submit_id=&i=</code> appended.</td></tr>
							<tr><td><code>context</code></td><td>The shared form context (<code>values</code>/<code>files</code>).</td></tr>
							<tr><td><code>options</code></td><td><code>{'{ headers, method }'}</code> forwarded to every request.</td></tr>
							<tr><td>Returns</td><td><code>Promise&lt;{'{ responses: [...] }'}&gt;</code> — one response per request (values + each file).</td></tr>
						</tbody>
					</table>
				</div>
			</div>
		</div>
	);
}
