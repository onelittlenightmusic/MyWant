# MyWant Frontend

A modern React-based SPA dashboard for managing MyWant configurations and executions.

## Features

- **Dashboard Overview**: Visual cards showing want status and statistics
- **Real-time Updates**: Live monitoring of want execution status
- **YAML Editor**: Syntax-highlighted editor for want configurations
- **Advanced Filtering**: Search and filter wants by status, type, and labels
- **Responsive Design**: Works on desktop, tablet, and mobile devices
- **Error Handling**: Comprehensive error boundaries and user feedback

## Technology Stack

- **React 18** with TypeScript
- **Vite** for fast development and building
- **Tailwind CSS** for styling
- **Zustand** for state management
- **CodeMirror 6** for YAML editing
- **Axios** for API communication
- **React Router** for navigation

## Development

### Prerequisites

- Node.js 18+
- npm or yarn
- MyWant backend server running on port 8080

### Setup

```bash
# Install dependencies
npm install

# Start development server
npm run dev

# Build for production
npm run build

# Preview production build
npm run preview
```

### Development Server

The development server runs on `http://localhost:8080` and proxies API requests to the MyWant backend at `http://localhost:8080`.

### Project Structure

```
src/
├── api/           # API client and types
├── components/    # React components
│   ├── common/    # Reusable components
│   ├── dashboard/ # Dashboard-specific components
│   ├── forms/     # Form components
│   ├── layout/    # Layout components
│   └── modals/    # Modal components
├── hooks/         # Custom React hooks
├── pages/         # Page components
├── stores/        # Zustand stores
├── styles/        # Global styles
├── types/         # TypeScript type definitions
└── utils/         # Utility functions
```

## API Integration

The frontend integrates with the MyWant REST API:

- `GET /api/v1/wants` - List all wants
- `POST /api/v1/wants` - Create new want
- `GET /api/v1/wants/{id}` - Get want details
- `PUT /api/v1/wants/{id}` - Update want
- `DELETE /api/v1/wants/{id}` - Delete want
- `GET /api/v1/wants/{id}/status` - Get execution status
- `GET /api/v1/wants/{id}/results` - Get execution results

## Configuration

Environment variables can be set in `.env.local`:

```bash
VITE_API_URL=http://localhost:8080
VITE_APP_TITLE=MyWant Dashboard
```

## Features

### Want Management
- Create wants using YAML configuration
- Edit existing want configurations
- View detailed want information
- Delete wants with confirmation
- Real-time status updates

### Dashboard
- Grid layout with want cards
- Status indicators and badges
- Execution timeline and duration
- Quick actions menu
- Search and filtering

### YAML Editor
- Syntax highlighting
- Auto-completion
- Validation feedback
- Format/beautify
- Sample templates

### Real-time Features
- Auto-refresh want status
- Live execution monitoring
- Polling-based updates
- Connection status indicators

## Deployment

### Production Build

```bash
npm run build
```

The build output goes to `dist/` directory.

### Serving Static Files

The MyWant backend serves static files from the `web/` directory. After building:

1. Copy `dist/*` contents to the `web/` directory in your MyWant installation
2. Start the MyWant server
3. Access the dashboard at `http://localhost:8080/`

### Docker Deployment

```dockerfile
# Build stage
FROM node:18-alpine as builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

# Production stage
FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/nginx.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
```

## Contributing

1. Follow TypeScript and React best practices
2. Use Tailwind CSS for styling
3. Add proper error handling
4. Include TypeScript types for new features
5. Test components thoroughly
6. Update documentation as needed

## Troubleshooting

### Common Issues

- **API Connection**: Ensure MyWant backend is running on port 8080
- **CORS Errors**: Backend should handle CORS for frontend origin
- **Build Errors**: Check Node.js version and dependency compatibility
- **TypeScript Errors**: Ensure all types are properly defined

### Development Tips

- Use browser dev tools to monitor API requests
- Check console for error messages and warnings
- Use React Developer Tools for component debugging
- Monitor network tab for API response issues