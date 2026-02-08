// Example: How to integrate market events into your frontend

/**
 * Fetch upcoming market events for a symbol
 * @param {string} symbol - Stock symbol (e.g., 'SPY', 'QQQ')
 * @returns {Promise<Object>} Events data
 */
async function getUpcomingEvents(symbol = 'SPY') {
    try {
        const response = await fetch(`/api/market-events/upcoming?symbol=${symbol}`);
        if (!response.ok) throw new Error('Failed to fetch events');
        const data = await response.json();
        return data;
    } catch (error) {
        console.error('Error fetching market events:', error);
        return { events: [], count: 0 };
    }
}

/**
 * Fetch market events in a date range
 * @param {string} symbol - Stock symbol
 * @param {string} startDate - Start date (YYYY-MM-DD)
 * @param {string} endDate - End date (YYYY-MM-DD)
 * @returns {Promise<Object>} Events data
 */
async function getEventsByDateRange(symbol = 'SPY', startDate, endDate) {
    try {
        const url = `/api/market-events/range?symbol=${symbol}&start=${startDate}&end=${endDate}`;
        const response = await fetch(url);
        if (!response.ok) throw new Error('Failed to fetch events');
        const data = await response.json();
        return data;
    } catch (error) {
        console.error('Error fetching market events:', error);
        return { events: [], count: 0 };
    }
}

/**
 * Display events in a table
 * @param {Array} events - Array of event objects
 * @param {string} containerId - DOM element ID to render into
 */
function displayEventsTable(events, containerId = 'market-events-table') {
    const container = document.getElementById(containerId);
    if (!container) return;

    // Get impact badge color
    const getImpactColor = (impact) => {
        switch(impact?.toLowerCase()) {
            case 'high': return 'bg-red-100 text-red-800';
            case 'medium': return 'bg-yellow-100 text-yellow-800';
            case 'low': return 'bg-green-100 text-green-800';
            default: return 'bg-gray-100 text-gray-800';
        }
    };

    const html = `
        <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200">
                <thead class="bg-gray-50">
                    <tr>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Date</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Event</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Impact</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Forecast</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Previous</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Actual</th>
                    </tr>
                </thead>
                <tbody class="bg-white divide-y divide-gray-200">
                    ${events.map(event => `
                        <tr>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                                ${event.EventDate}
                                ${event.EventTime ? `<br><span class="text-xs text-gray-500">${event.EventTime}</span>` : ''}
                            </td>
                            <td class="px-6 py-4 text-sm text-gray-900">
                                <div class="font-medium">${event.Title}</div>
                                ${event.Description ? `<div class="text-xs text-gray-500">${event.Description}</div>` : ''}
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap">
                                ${event.Impact ? `<span class="px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${getImpactColor(event.Impact)}">${event.Impact}</span>` : '-'}
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">${event.Forecast || '-'}</td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">${event.Previous || '-'}</td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">${event.Actual || '-'}</td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        </div>
    `;

    container.innerHTML = events.length > 0 ? html : '<p class="text-gray-500 text-center py-4">No events found</p>';
}

/**
 * Create an events calendar widget
 * @param {Array} events - Array of event objects
 * @param {string} containerId - DOM element ID to render into
 */
function displayEventsCalendar(events, containerId = 'events-calendar') {
    const container = document.getElementById(containerId);
    if (!container) return;

    // Group events by date
    const eventsByDate = events.reduce((acc, event) => {
        const date = event.EventDate;
        if (!acc[date]) acc[date] = [];
        acc[date].push(event);
        return acc;
    }, {});

    const html = `
        <div class="space-y-4">
            ${Object.entries(eventsByDate).map(([date, dayEvents]) => `
                <div class="border rounded-lg p-4">
                    <h3 class="font-bold text-lg mb-2">${date}</h3>
                    <div class="space-y-2">
                        ${dayEvents.map(event => `
                            <div class="border-l-4 ${event.Impact === 'High' ? 'border-red-500' : event.Impact === 'Medium' ? 'border-yellow-500' : 'border-green-500'} pl-3">
                                <div class="font-medium">${event.Title}</div>
                                ${event.Description ? `<div class="text-sm text-gray-600">${event.Description}</div>` : ''}
                                ${event.EventTime ? `<div class="text-xs text-gray-500">${event.EventTime}</div>` : ''}
                            </div>
                        `).join('')}
                    </div>
                </div>
            `).join('')}
        </div>
    `;

    container.innerHTML = html;
}

// Example usage:
document.addEventListener('DOMContentLoaded', async () => {
    // Load upcoming events for SPY
    const data = await getUpcomingEvents('SPY');
    console.log(`Loaded ${data.count} upcoming events for ${data.symbol}`);
    
    // Display in table
    displayEventsTable(data.events, 'market-events-table');
    
    // Or display as calendar
    // displayEventsCalendar(data.events, 'events-calendar');
    
    // Load events for current week
    const today = new Date().toISOString().split('T')[0];
    const nextWeek = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString().split('T')[0];
    const weekData = await getEventsByDateRange('SPY', today, nextWeek);
    console.log(`This week: ${weekData.count} events`);
});

// Utility: Filter high-impact events
function getHighImpactEvents(events) {
    return events.filter(e => e.Impact === 'High');
}

// Utility: Get events for today
function getTodayEvents(events) {
    const today = new Date().toISOString().split('T')[0];
    return events.filter(e => e.EventDate === today);
}

// Utility: Create event notification
function notifyHighImpactEvents(events) {
    const highImpact = getHighImpactEvents(events);
    if (highImpact.length > 0 && 'Notification' in window) {
        Notification.requestPermission().then(permission => {
            if (permission === 'granted') {
                new Notification(`${highImpact.length} High Impact Events Coming`, {
                    body: highImpact.map(e => e.Title).join(', '),
                    icon: '/static/images/alert-icon.png'
                });
            }
        });
    }
}

// Export functions for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        getUpcomingEvents,
        getEventsByDateRange,
        displayEventsTable,
        displayEventsCalendar,
        getHighImpactEvents,
        getTodayEvents,
        notifyHighImpactEvents
    };
}
