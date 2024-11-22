class XPartial {
    constructor(options = {}) {
        // Define the custom action attributes
        this.actionAttributes = ['x-get', 'x-post', 'x-put', 'x-delete'];
        // Optionally, allow extending action attributes via options
        if (options.additionalAttributes) {
            this.actionAttributes.push(...options.additionalAttributes);
        }

        // Default swap option: 'outerHTML' or 'innerHTML'
        this.defaultSwapOption = options.defaultSwapOption || 'outerHTML'; // Can be overridden per element

        // Bind methods to ensure correct 'this' context
        this.scanForElements = this.scanForElements.bind(this);
        this.setupElement = this.setupElement.bind(this);
        this.handleAction = this.handleAction.bind(this);
        this.handleOobSwapping = this.handleOobSwapping.bind(this);

        // Initialize the handler on DOMContentLoaded using an arrow function to avoid passing the event object
        document.addEventListener('DOMContentLoaded', () => this.scanForElements());
    }

    /**
     * Scans the entire document or a specific container for elements with defined action attributes.
     * @param {HTMLElement | Document} [container=document]
     */
    scanForElements(container = document) {
        const selector = this.actionAttributes.map(attr => `[${attr}]`).join(',');
        const elements = container.querySelectorAll(selector);
        elements.forEach(this.setupElement);
    }

    /**
     * Sets up an individual element by attaching the appropriate event listener.
     * @param {HTMLElement} element
     */
    setupElement(element) {
        const trigger = element.getAttribute('x-trigger') || 'click';
        // Avoid attaching multiple listeners
        if (!element.__xRequestHandlerInitialized) {
            element.addEventListener(trigger, (event) => {
                event.preventDefault();
                this.handleAction(event, element);
            });
            // Mark the element as initialized
            element.__xRequestHandlerInitialized = true;
        }
    }

    /**
     * Handles the action when an element is triggered.
     * @param {Event} event
     * @param {HTMLElement} element
     */
    async handleAction(event, element) {
        const method = this.getMethod(element);
        const url = element.getAttribute(`x-${method.toLowerCase()}`);

        if (!url) {
            console.error(`No URL specified for method ${method} on element:`, element);
            return;
        }

        const headers = this.getHeaders(element); // Includes X-Partial
        const partialId = element.getAttribute('x-partial'); // Replaces x-target
        const select = element.getAttribute('x-select'); // For UI state management

        if (!partialId) {
            console.error(`Element does not have 'x-partial' attribute:`, element);
            return;
        }

        const partialSelector = `#${partialId}`;
        const targetElement = document.querySelector(partialSelector);

        if (!targetElement) {
            console.error(`No element found with ID '${partialId}' for 'x-partial' targeting.`);
            return;
        }

        try {
            const responseText = await this.performRequest(method, url, headers, element);

            // Parse the response HTML
            const parser = new DOMParser();
            const doc = parser.parseFromString(responseText, 'text/html');

            // Extract OOB elements
            const oobElements = Array.from(doc.querySelectorAll('[x-swap-oob]'));
            // Remove OOB elements from the main content to prevent duplication
            oobElements.forEach(el => el.parentNode.removeChild(el));

            // Get the remaining HTML as the main content
            const mainContent = doc.body.innerHTML;

            // Replace the target's innerHTML with main content
            targetElement.innerHTML = mainContent;
            // Re-scan the newly added content for XRequestHandler elements
            this.scanForElements(targetElement);

            // Handle OOB swapping with the extracted OOB elements
            this.handleOobSwapping(oobElements);

            // Handle 'x-select' attribute if present
            if (select) {
                this.handleSelect(select, element);
            }
        } catch (error) {
            console.error('Request failed:', error);
            // Optionally, handle error display in the UI
            targetElement.innerHTML = `<div class="error">An error occurred: ${error.message}</div>`;
        }
    }

    /**
     * Determines the HTTP method based on the element's attributes.
     * @param {HTMLElement} element
     * @returns {string} HTTP method
     */
    getMethod(element) {
        for (const attr of this.actionAttributes) {
            if (element.hasAttribute(attr)) {
                return attr.replace('x-', '').toUpperCase();
            }
        }
        return 'GET'; // Default method
    }

    /**
     * Constructs headers from the element's attributes.
     * @param {HTMLElement} element
     * @returns {Object} Headers object
     */
    getHeaders(element) {
        const headers = {};
        const action = element.getAttribute('x-action');
        const select = element.getAttribute('x-select');
        const partial = element.getAttribute('x-partial'); // Used for frontend targeting

        if (action) headers['X-Action'] = action;
        if (select) headers['X-Select'] = select;
        if (partial) headers['X-Partial'] = partial; // Send to backend without '#'

        // Add any additional custom headers if needed
        const customHeaders = element.getAttribute('x-headers');
        if (customHeaders) {
            try {
                const parsedHeaders = JSON.parse(customHeaders);
                Object.assign(headers, parsedHeaders);
            } catch (e) {
                console.error('Invalid JSON in x-headers:', e);
            }
        }

        return headers;
    }

    /**
     * Performs the HTTP request using Fetch API.
     * @param {string} method
     * @param {string} url
     * @param {Object} headers
     * @param {HTMLElement} element
     * @returns {Promise<string>} Response text
     */
    performRequest(method, url, headers, element) {
        const options = {
            method,
            headers,
            credentials: 'same-origin',
        };

        if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
            let body = null;

            if (element.tagName === 'FORM') {
                body = new FormData(element);
            } else {
                const closestForm = element.closest('form');
                if (closestForm) {
                    body = new FormData(closestForm);
                }
            }

            if (body) {
                options.body = body;
            }
        }

        return fetch(url, options).then(response => {
            if (!response.ok) {
                return response.text().then(text => {
                    throw new Error(`HTTP error ${response.status}: ${text}`);
                });
            }
            return response.text();
        });
    }

    /**
     * Handles the 'x-select' attribute if used for additional logic.
     * This method can be customized based on what 'x-select' is intended to do.
     * @param {string} select
     * @param {HTMLElement} element
     */
    handleSelect(select, element) {
        // Example implementation: Add a 'selected' class to the element
        const selectedElements = document.querySelectorAll(`[x-select="${select}"]`);
        selectedElements.forEach(el => {
            el.classList.remove('selected');
        });
        // Add 'selected' class to the current element
        element.classList.add('selected');
    }

    /**
     * Handles Out-of-Band (OOB) swapping by processing an array of OOB elements.
     * Replaces existing elements in the document with the new content based on matching IDs.
     * @param {HTMLElement[]} oobElements
     */
    handleOobSwapping(oobElements) {
        oobElements.forEach(oobElement => {
            const targetId = oobElement.getAttribute('id');
            if (!targetId) {
                console.error('OOB element does not have an ID:', oobElement);
                return;
            }

            const swapOption = oobElement.getAttribute('x-swap-oob') || this.defaultSwapOption;
            const existingElement = document.getElementById(targetId);

            if (!existingElement) {
                console.error(`No existing element found with ID '${targetId}' for OOB swapping.`);
                return;
            }

            if (swapOption === 'outerHTML') {
                existingElement.outerHTML = oobElement.outerHTML;
            } else if (swapOption === 'innerHTML') {
                existingElement.innerHTML = oobElement.innerHTML;
            } else {
                console.error(`Invalid x-swap-oob option '${swapOption}' on element with ID '${targetId}'. Use 'outerHTML' or 'innerHTML'.`);
                return;
            }

            // After swapping, initialize any new elements within the replaced content
            const newElement = document.getElementById(targetId);
            if (newElement) {
                this.scanForElements(newElement);
            }
        });
    }

    /**
     * Allows manually re-scanning a specific container for XRequestHandler elements.
     * Useful when dynamically adding content to the DOM.
     * @param {HTMLElement} container
     */
    refresh(container = document) {
        this.scanForElements(container);
    }
}

// Initialize the handler with optional configuration
const xRequestHandler = new XRequestHandler({
    defaultSwapOption: 'outerHTML' // Default swap option: 'outerHTML' or 'innerHTML'
});
