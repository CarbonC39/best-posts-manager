## Frontend Development Standards

### 1. Zero Superfluous Microcopy (Eliminate Visual Noise)
* **Rule**: Never render patronizing, defensive, or stating-the-obvious helper text. If an action is standard and predictable, do not clutter the viewport with reassurance notices.
* **Negative Examples (Banned)**:
  * ❌ `"Data has been saved locally"` (when auto-save is default behaviour)
  * ❌ `"Changes will only be published after preview"`
  * ❌ `"Click here to submit the form"`
* **Allowed Feedback**: Reserve system messaging strictly for asynchronous states (loading spinners), actionable errors, or high-friction destructive confirmations (e.g., delete modals).

---

### 2. Strong Typographic Hierarchy & Readability Floors
* **Rule**: Maintain pronounced visual hierarchy across heading levels.
  * **Heading Scale**: Any heading (`h1`, `h2`, `h3`) must be **at least 1.5×** the font size of the immediate body text it introduces.
  * **Minimum Legibility**: Never use font sizes below `10px` under any circumstances. Standard body text must adhere to platform conventions (minimum `12px`–`14px`), with `10px` reserved strictly for compact tags or metadata badges.

---

### 3. Backend-Driven UI & Data Contract Fidelity (No Phantom Features)
* **Rule**: UI layouts and components must strictly reflect actual backend capabilities, API schemas, and data models. Never invent UI affordances purely for visual balance, symmetry, or aesthetic filler.
* **Negative Examples (Banned)**:
  * ❌ Adding an avatar placeholder/upload slot when the user schema has no `avatar_url` or profile image support.
  * ❌ Adding social share counts or star ratings when no corresponding backend metrics exist.
* **Standard**: If a field or feature is absent from the backend contract, do not render UI placeholders or mock controls for it.

---

### 4. Modular Scoping & Semantic Hierarchy
* **Rule**: Maintain strict logical cohesion between parent containers and their child elements. Elements within a component must share a direct, logical dependency.
* **Structural Guidelines**:
  * Encapsulate related UI elements together (e.g., notification icons and alert copy must coexist inside a dedicated alert module).
  * **Anti-Pattern (Banned)**: Placing contextual descriptions or foreign instructions inside a button-specific wrapper container (`<div class="btn-group">`).
  * Ensure HTML markup is semantic (`<main>`, `<section>`, `<article>`, `<button>`, `<aside>`) rather than arbitrary nested `<div>` soup.

---

### 5. Single Source of Action (DRY Interface)
* **Rule**: Every distinct user action or core piece of information must appear once and only once per viewport context.
* **Negative Examples (Banned)**:
  * ❌ Rendering a "Publish" button at both the top toolbar and bottom footer without explicit pagination/long-scroll requirements.
  * ❌ Repeating the same error notice in both a banner and an inline toast simultaneously.
* **Standard**: Provide a single, canonical primary CTA per screen flow.

---

### 6. Iconography & High-Information Scannability
* **Rule**: Pair primary interactive elements and key metrics with recognizable iconography (e.g., Lucide, Heroicons, SVG) to enhance scannability and reduce cognitive load.
* **Anti-Pattern**: Avoid wide, plain-text-only buttons, pill tags, or dense spans that force the user to read verbose paragraphs to understand basic actions.
* **Guideline**: Keep button labels concise (1–3 words) and visually anchored by an icon.

---

### 7. Utility-First Design (Function Over Ornamentation)
* **Rule**: Design every component with meaningful intent. Visual appeal must be an emergent property of clarity, functional layout, and content hierarchy—not decorative vanity.
* **Banned Patterns**:
  * ❌ Adding empty decorative cards, artificial dividers, or ornamental containers solely to "balance the grid".
  * ❌ Using complex gradients, pulsing animations, or drop shadows where a simple neutral border conveys state more clearly.

---

### 8. Semantic & Affordance Integrity
* **Rule**: Visual styles must strictly correspond to their recognized interactive affordances and standard UI semantics.
* **Negative Examples (Banned)**:
  * ❌ Styling an active/selected radio or list item using Markdown quote block styling (`<blockquote>` or thick left accent bar intended for citations).
  * ❌ Styling static informational callouts to look like clickable buttons or input fields.
* **Standard**: Use designated design tokens for selected, hovered, disabled, and active states.

---

### 9. Scope Containment & Non-Destructive Edits
* **Rule**: Strictly modify only the code, files, and components explicitly requested.
* **Anti-Patterns (Banned)**:
  * ❌ Unsolicited refactoring of adjacent code, reordering imports, changing global CSS/Tailwind configs, or altering unrelated components.
  * ❌ Re-architecting working logic under the guise of "code cleanup".
* **Standard**: Confine all edits to the minimum required blast radius.
