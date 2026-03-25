
# Technical Documentation

This document provides a detailed overview of the project's technical architecture, development practices, and core functionalities. It is intended to be a guide for developers working on this project.

## 1. Project Architecture

This project is a full-stack web application built with [Next.js](https://nextjs.org/), a React framework for building server-side rendered and statically generated web applications.

- **Framework**: Next.js 15.4.3
- **Language**: TypeScript
- **Authentication**: NextAuth.js v5
- **Database**: PostgreSQL with Prisma ORM
- **Styling**: Tailwind CSS 4
- **Internationalization**: `next-intl`
- **Form Management**: React Hook Form + Zod
- **File Storage**: Aliyun OSS
- **Linting**: ESLint

### 1.1. Directory Structure

The project follows a standard Next.js `src` directory structure:

```
/
├── prisma/           # Prisma schema, migrations, and seed scripts
├── public/           # Static assets (including game templates)
├── src/
│   ├── app/          # Next.js App Router
│   │   ├── api/      # API routes
│   │   ├── auth/     # Authentication pages
│   │   ├── admin/    # Admin dashboard
│   │   ├── account/  # User account pages
│   │   ├── play/     # Game player pages
│   │   └── projects/# Project pages
│   ├── components/   # Reusable React components
│   │   ├── ui/       # Base UI components
│   │   ├── forms/    # Form components
│   │   └── ...       # Business components
│   ├── lib/          # Core libraries and helper functions
│   ├── messages/     # Language-specific message files
│   ├── actions/      # Server Actions
│   └── middleware.ts # Request middleware
├── .env.local        # Local environment variables
├── next.config.ts    # Next.js configuration
└── package.json      # Project dependencies and scripts
```

## 2. Package Versions

The following are the key dependencies and their versions as defined in `package.json`:

- **next**: `15.4.3`
- **react**: `19.1.0`
- **typescript**: `5.7.3`
- **next-auth**: `5.0.0-beta.29`
- **@prisma/client**: `6.12.0`
- **next-intl**: `4.3.4`
- **tailwindcss**: `4.0.0`
- **@headlessui/react**: `2.2.7`
- **react-hook-form**: `7.61.1`
- **zod**: `4.0.10`

For a complete list of dependencies, refer to the `package.json` file.

## 3. Environment Variables

The application requires the following environment variables to be set in a `.env.local` file at the root of the project.

```bash
# Database
DATABASE_URL="postgresql://USER:PASSWORD@HOST:PORT/DATABASE"

# NextAuth.js
AUTH_SECRET="your-super-secret-auth-secret" # Generate with `openssl rand -hex 32`
AUTH_URL="http://localhost:3000"

# GitHub OAuth (optional)
AUTH_GITHUB_ID="your-github-id"
AUTH_GITHUB_SECRET="your-github-secret"

# Email (Resend)
RESEND_API_KEY="your-resend-api-key"
```

## 4. Authentication (NextAuth.js)

Authentication is handled by [NextAuth.js](https://next-auth.js.org/). The configuration is split across several files for better organization.

### 4.1. Configuration (`src/auth.config.ts`)

This file contains the main NextAuth.js configuration, including providers and callbacks.

```typescript
// src/auth.config.ts
import type { NextAuthConfig } from 'next-auth';
import Credentials from 'next-auth/providers/credentials';
import GitHub from 'next-auth/providers/github';
import { LoginSchema } from '@/lib/validations';
import prisma from '@/lib/prisma';
import bcrypt from 'bcryptjs';

export const authConfig = {
  // ... (providers and callbacks configuration)
} satisfies NextAuthConfig;
```

### 4.2. Session Management (`src/lib/auth.ts`)

This file exports the `auth` and `handlers` objects from NextAuth.js, which are used to get the session on the server-side and to create the API routes for authentication.

```typescript
// src/lib/auth.ts
import NextAuth from 'next-auth';
import { authConfig } from '@/auth.config';

export const {
  handlers: { GET, POST },
  auth,
  signIn,
  signOut,
} = NextAuth(authConfig);
```

### 4.3. Usage in API Routes and Components

To get the current user session in an API route or server component, use the `auth()` function:

```typescript
// src/app/api/some-protected-route/route.ts
import { auth } from '@/lib/auth';
import { NextResponse } from 'next/server';

export async function GET() {
  const session = await auth();
  if (!session?.user) {
    return NextResponse.json({ error: 'Unauthorized' }, { status: 401 });
  }
  // ... your logic
}
```

## 5. Internationalization (i18n)

Internationalization is implemented using `next-intl`.

### 5.1. Configuration (`src/i18n/request.ts`)

This file configures `next-intl` to determine the current locale from the user's cookies.

```typescript
// src/i18n/request.ts
import { getRequestConfig } from 'next-intl/server';
import { cookies } from 'next/headers';

export default getRequestConfig(async () => {
  const cookieStore = await cookies();
  const cookieLocale = cookieStore.get('NEXT_LOCALE')?.value;
  const locale = cookieLocale || 'en';

  return {
    locale,
    messages: (await import(`../messages/${locale}.json`)).default,
  };
});
```

### 5.2. Middleware (`src/middleware.ts`)

The middleware is responsible for redirecting users to the correct locale-prefixed path.

```typescript
// src/middleware.ts
import { NextRequest, NextResponse } from 'next/server';
import { i18nRouter } from 'next-i18n-router';
import { i18nConfig } from '../i18nConfig';

export function middleware(request: NextRequest) {
  return i18nRouter(request, i18nConfig);
}

export const config = {
  matcher: '/((?!api|static|.*\..*|_next).*)',
};
```

### 5.3. Usage in Components

Use the `useTranslations` hook from `next-intl` to access translations in your components.

```typescript
// src/components/SomeComponent.tsx
import { useTranslations } from 'next-intl';

export default function SomeComponent() {
  const t = useTranslations('SomeComponent');
  return <h1>{t('title')}</h1>;
}
```

## 6. Error Handling

The project uses high-order functions (HOFs) to handle errors in API routes gracefully.

### 6.1. Error Handlers (`src/lib/errorHandler.ts`)

There are two main error handlers:

- `withStaticErrHandle`: For API routes without dynamic parameters.
- `withNewErrHandle`: For API routes with dynamic parameters (e.g., `[id]`).

### 6.2. Usage

Wrap your API route handlers with the appropriate error handler.

**Static Route Example:**

```typescript
// src/app/api/projects/route.ts
import { withStaticErrHandle } from '@/lib/errorHandler';
import { NextRequest, NextResponse } from 'next/server';

export const GET = withStaticErrHandle(async (req: NextRequest) => {
  // ... your logic
  return NextResponse.json({ message: 'Success' });
});
```

**Dynamic Route Example:**

```typescript
// src/app/api/projects/[id]/route.ts
import { withNewErrHandle } from '@/lib/errorHandler';
import { NextRequest, NextResponse } from 'next/server';

export const GET = withNewErrHandle(async (req: NextRequest, { params }: { params: { id: string } }) => {
  const { id } = params;
  // ... your logic
  return NextResponse.json({ message: `Success for ID: ${id}` });
});
```

## 7. API Route Definitions

API routes are defined within the `src/app/api` directory using the Next.js App Router conventions.

### 7.1. `GET` Request

Here is an example of a `GET` request that fetches a list of projects.

```typescript
// src/app/api/projects/route.ts
import { NextRequest, NextResponse } from 'next/server';
import prisma from '@/lib/prisma';
import { withStaticErrHandle } from '@/lib/errorHandler';

export const GET = withStaticErrHandle(async (req: NextRequest) => {
  const projects = await prisma.project.findMany({
    // ... query options
  });
  return NextResponse.json(projects);
});
```

### 7.2. `POST` Request

Here is an example of a `POST` request that creates a "like" for a project.

```typescript
// src/app/api/projects/[id]/like/route.ts
import { NextRequest, NextResponse } from 'next/server';
import prisma from '@/lib/prisma';
import { withNewErrHandle } from '@/lib/errorHandler';
import { auth } from '@/lib/auth';

export const POST = withNewErrHandle(async (req: NextRequest, { params }: { params: Promise<{ id: string }> }) => {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: 'Unauthorized' }, { status: 401 });
  }

  const { id: projectId } = await params;
  const userId = parseInt(session.user.id, 10);

  // ... logic to create a like
  
  return NextResponse.json({ success: true });
});
```

## 8. Email Service

The application uses [Resend](https://resend.com/) to send emails. The email service is configured in `src/lib/email.ts`.

### 8.1. Configuration

Ensure your `RESEND_API_KEY` is set in your environment variables.

### 8.2. Sending Emails

The `sendEmail` function can be used to send emails. It takes the recipient's email, subject, and the React component for the email body as arguments.

```typescript
// src/lib/email.ts
import { Resend } from 'resend';
import { EmailTemplate } from '@/components/EmailTemplate'; // Example email template

const resend = new Resend(process.env.RESEND_API_KEY);

export const sendEmail = async (to: string, subject: string, text: string) => {
  try {
    await resend.emails.send({
      from: 'Your Name <your-email@example.com>',
      to,
      subject,
      react: EmailTemplate({ text }), // Using a React component for the email body
    });
  } catch (error) {
    console.error('Error sending email:', error);
  }
};
```

### 8.3. Usage Example

```typescript
// some-action.ts
import { sendEmail } from '@/lib/email';

export async function someAction() {
  // ...
  await sendEmail('user@example.com', 'Welcome!', 'Thank you for signing up.');
  // ...
}
```

## 9. Form Management with React Hook Form

The project uses [React Hook Form](https://react-hook-form.com/) for managing form state and validation, coupled with [Zod](https://zod.dev/) for schema validation.

### 9.1. Schema Definition

Define a validation schema using Zod. This schema will be the single source of truth for your form's data structure and validation rules.

```typescript
// src/lib/validations.ts
import { z } from 'zod';

export const LoginSchema = z.object({
  email: z.string().email({ message: 'Please enter a valid email address.' }),
  password: z.string().min(6, { message: 'Password must be at least 6 characters.' }),
});
```

### 9.2. Form Component Implementation

Use the `useForm` hook in your component to manage the form.

```typescript
// src/components/LoginForm.tsx
'use client';

import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { LoginSchema } from '@/lib/validations';
import type { z } from 'zod';

type FormData = z.infer<typeof LoginSchema>;

export function LoginForm() {
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormData>({
    resolver: zodResolver(LoginSchema),
  });

  const onSubmit = (data: FormData) => {
    // Handle form submission
    console.log(data);
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)}>
      <div>
        <label htmlFor="email">Email</label>
        <input id="email" type="email" {...register('email')} />
        {errors.email && <p>{errors.email.message}</p>}
      </div>
      <div>
        <label htmlFor="password">Password</label>
        <input id="password" type="password" {...register('password')} />
        {errors.password && <p>{errors.password.message}</p>}
      </div>
      <button type="submit">Login</button>
    </form>
  );
}
```

## 10. Database Migration Workflow with Prisma

Database schema changes are managed by [Prisma Migrate](https://www.prisma.io/docs/concepts/components/prisma-migrate).

### 10.1. Development Workflow

1.  **Modify Schema**: Make changes to your `prisma/schema.prisma` file.
2.  **Generate Migration**: Run the following command to generate a new SQL migration file. This command will also apply the migration to your local development database.

    ```bash
    npx prisma migrate dev --name <migration-name>
    ```
    Replace `<migration-name>` with a descriptive name for your changes (e.g., `add-user-role`).

3.  **Generate Prisma Client**: After the migration, it's crucial to update the Prisma Client to reflect the new schema.

    ```bash
    npx prisma generate
    ```

### 10.2. Production Workflow

In a production environment, you should only apply existing migrations.

```bash
npx prisma migrate deploy
```

**Important**: Never manually edit the generated SQL files inside the `prisma/migrations` directory.

## 11. UI Components with Headless UI

The project leverages [Headless UI](https://headlessui.com/) for building fully accessible, unstyled UI components. This allows for custom styling with Tailwind CSS while maintaining accessibility best practices.

### 11.1. Example: A Custom Dropdown Menu

This example uses Headless UI's `Menu` component to create a dropdown.

```typescript
// src/components/UserDropdown.tsx
import { Menu, MenuButton, MenuItem, MenuItems } from '@headlessui/react';
import { ChevronDownIcon } from '@heroicons/react/20/solid';

export function UserDropdown() {
  return (
    <Menu as="div" className="relative">
      <MenuButton className="inline-flex items-center gap-2">
        <span>Options</span>
        <ChevronDownIcon className="h-5 w-5" />
      </MenuButton>
      <MenuItems
        anchor="bottom"
        className="w-52 origin-top-right rounded-xl border bg-white p-1"
      >
        <MenuItem>
          <a className="block p-2 data-[focus]:bg-blue-100" href="/settings">
            Settings
          </a>
        </MenuItem>
        <MenuItem>
          <a className="block p-2 data-[focus]:bg-blue-100" href="/logout">
            Sign out
          </a>
        </MenuItem>
      </MenuItems>
    </Menu>
  );
}
```

This approach separates logic (from Headless UI) from presentation (styled with Tailwind CSS), leading to more maintainable and flexible components.

## 12. Login and Authentication Flow

The project implements both credentials-based (email/password) and third-party social logins.

### 12.1. Credentials Login Flow

The email and password login is handled via a Server Action, providing a seamless experience without a full page reload.

1.  **Client-Side Call**: The login form calls the `authenticate` server action.

    ```typescript
    // Simplified example from a login form component
    import { authenticate } from '@/actions/auth';

    async function handleLogin(formData: FormData) {
      const result = await authenticate(Object.fromEntries(formData));
      if (result?.error) {
        // Handle and display the error
      }
    }
    ```

2.  **Server Action (`src/actions/auth.ts`)**: The `authenticate` action validates the input using the `LoginSchema` and then calls the `signIn` function from NextAuth.js with the `credentials` provider.

    ```typescript
    // src/actions/auth.ts
    export async function authenticate(values: z.infer<typeof LoginSchema>) {
      // ... validation ...
      try {
        await signIn('credentials', {
          email,
          password,
          redirectTo: '/',
        });
        return { success: 'Logged in!' };
      } catch (error) {
        if (error instanceof AuthError) {
          // ... error handling ...
        }
        throw error;
      }
    }
    ```

3.  **NextAuth.js Provider**: The `Credentials` provider in the NextAuth.js configuration (likely located in a file not included in the edge-safe `auth.config.ts`, e.g., `auth.ts`) receives the credentials, finds the user in the database, and verifies the password.

### 12.2. Third-Party Login Flow

The third-party login flow uses a custom implementation that involves a client-side call to a dedicated API route, which then provides the redirect URL.

1.  **Client Component (`src/components/ThirdPartyLogin.tsx`)**: The user clicks a button, which triggers the `handleLogin` function.

    ```typescript
    // src/components/ThirdPartyLogin.tsx
    const handleLogin = async () => {
      setIsLoading(true);
      try {
        // 1. Call the custom API endpoint
        const response = await fetch(`/api/auth/third-party/login?type=${type}`);
        const data = await response.json();

        if (data.url) {
          // 2. Redirect the browser to the provider's auth URL
          window.location.href = data.url;
        }
      } catch (error) {
        console.error('Login error:', error);
      } finally {
        setIsLoading(false);
      }
    };
    ```

2.  **Backend API Route (`/api/auth/third-party/login`)**: This API route (not shown in the provided files, but its existence is implied) receives the `type` (e.g., 'github', 'google'), generates the appropriate provider-specific authorization URL, and returns it as a JSON response.

3.  **NextAuth.js Callback**: After the user authenticates with the third party, the provider redirects back to the application's standard NextAuth.js callback URL (`/api/auth/callback/[provider]`). NextAuth.js then completes the sign-in process, creates a new user if one doesn't exist, and establishes a session.

This custom flow allows the backend to control the generation of the authorization URL, potentially adding custom parameters or logic before the user is redirected.

### 12.3. User Registration

New users are registered via the `register` server action.

1.  **Schema and Validation**: Uses the `RegisterSchema` from `zod` for validation.
2.  **Password Hashing**: Hashes the user's password using `bcryptjs` before storing it.
3.  **Database Interaction**: Checks if a user with the same email already exists and, if not, creates a new user in the database using Prisma.
4.  **Email Verification**: Generates a verification token and sends a confirmation email to the user.

    ```typescript
    // src/actions/auth.ts
    export async function register(values: z.infer<typeof RegisterSchema>) {
      // ... validation ...

      const hashedPassword = await bcrypt.hash(password, 10);

      // ... check for existing user ...

      await prisma.user.create({
        data: {
          name,
          email,
          password: hashedPassword,
        },
      });

      const verificationToken = await generateVerificationToken(email);
      await sendVerificationEmail(
        verificationToken.email,
        verificationToken.token,
      );

      return { success: 'Confirmation email sent!' };
    }
    ```

