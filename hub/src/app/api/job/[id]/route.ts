import { NextRequest, NextResponse } from 'next/server';
import { prisma } from '@/lib/prisma';

export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  try {
    const { id } = await params;

    // Fetch job with email checks
    const job = await prisma.job.findUnique({
      where: { id },
      include: {
        emailChecks: {
          orderBy: {
            email: 'asc',
          },
        },
      },
    });

    if (!job) {
      return NextResponse.json(
        { error: 'Job not found' },
        { status: 404 }
      );
    }

    return NextResponse.json({
      id: job.id,
      totalEmails: job.totalEmails,
      status: job.status,
      createdAt: job.createdAt,
      emailChecks: job.emailChecks.map((check) => ({
        id: check.id,
        email: check.email,
        status: check.status,
        smtpCode: check.smtpCode,
        bounceReason: check.bounceReason,
      })),
    });
  } catch (error) {
    console.error('Error fetching job:', error);
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }
}
