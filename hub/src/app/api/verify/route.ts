import { NextRequest, NextResponse } from 'next/server';
import { prisma } from '@/lib/prisma';
import { getRedisClient } from '@/lib/redis';

export async function POST(request: NextRequest) {
  try {
    const body = await request.json();
    
    // Validate input
    if (!Array.isArray(body) || body.length === 0) {
      return NextResponse.json(
        { error: 'Expected a non-empty array of email addresses' },
        { status: 400 }
      );
    }

    // Validate emails are strings
    const emails = body.filter((email) => typeof email === 'string' && email.trim().length > 0);
    
    if (emails.length === 0) {
      return NextResponse.json(
        { error: 'No valid email addresses provided' },
        { status: 400 }
      );
    }

    // Create job in database
    const job = await prisma.job.create({
      data: {
        totalEmails: emails.length,
        status: 'PENDING',
        emailChecks: {
          create: emails.map((email) => ({
            email: email.trim(),
            status: 'PENDING',
          })),
        },
      },
    });

    // Push emails to Redis queue
    const redis = getRedisClient();
    const queueData = emails.map((email) => JSON.stringify({ jobId: job.id, email: email.trim() }));
    
    if (queueData.length > 0) {
      await redis.lpush('email_queue', ...queueData);
    }

    return NextResponse.json(
      {
        jobId: job.id,
        totalEmails: emails.length,
        status: job.status,
      },
      { status: 201 }
    );
  } catch (error) {
    console.error('Error creating verification job:', error);
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }
}
