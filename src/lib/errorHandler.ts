import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

export class ApiError extends Error {
  constructor(
    public statusCode: number,
    public message: string,
    public data?: unknown
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export function withStaticErrHandle(handler: (req: NextRequest) => Promise<unknown>) {
  return async (req: NextRequest) => {
    try {
      return await handler(req);
    } catch (error) {
      console.error('API Error:', error);
      
      if (error instanceof ApiError) {
        return NextResponse.json(
          { error: error.message, data: error.data },
          { status: error.statusCode }
        );
      }
      
      return NextResponse.json(
        { error: 'Internal Server Error' },
        { status: 500 }
      );
    }
  };
}

export function withNewErrHandle(handler: (req: NextRequest, context: unknown) => Promise<unknown>) {
  return async (req: NextRequest, context: unknown) => {
    try {
      return await handler(req, context);
    } catch (error) {
      console.error('API Error:', error);
      
      if (error instanceof ApiError) {
        return NextResponse.json(
          { error: error.message, data: error.data },
          { status: error.statusCode }
        );
      }
      
      return NextResponse.json(
        { error: 'Internal Server Error' },
        { status: 500 }
      );
    }
  };
}